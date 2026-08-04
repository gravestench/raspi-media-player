package app

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/dylanknuth/raspi-media-player/internal/auth"
)

const sessionCookieName = "jukebox_session"
const csrfCookieName = "jukebox_csrf"

type identityKey struct{}
type Identity struct{ Session auth.Session }

func (i *Identity) User() auth.User { return i.Session.User }
func identityFromContext(ctx context.Context) *Identity {
	value, _ := ctx.Value(identityKey{}).(*Identity)
	return value
}

type credentialsRequest struct {
	Username             string `json:"username"`
	Password             string `json:"password"`
	PasswordConfirmation string `json:"password_confirmation"`
}

func (a *application) authenticate(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		cookie, err := r.Cookie(sessionCookieName)
		if err == nil {
			session, resolveErr := a.auth.ResolveSession(r.Context(), cookie.Value)
			if resolveErr == nil {
				identity := &Identity{Session: session}
				ctx := context.WithValue(r.Context(), identityKey{}, identity)
				if metadata, ok := ctx.Value(requestMetadataKey{}).(*requestMetadata); ok {
					metadata.userID = session.User.ID
					metadata.username = session.User.Username
				}
				if logger := loggerFromContext(ctx, a.logger); logger != nil {
					ctx = context.WithValue(ctx, requestLoggerKey{}, logger.With("user_id", session.User.ID, "username", session.User.Username))
				}
				r = r.WithContext(ctx)
			} else if !errors.Is(resolveErr, auth.ErrInvalidSession) {
				loggerFromContext(r.Context(), a.logger).Error("session lookup failed", "error", resolveErr)
			}
		}
		next.ServeHTTP(w, r)
	})
}

func (a *application) protectMutations(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		identity := identityFromContext(r.Context())
		mutation := r.Method == http.MethodPost || r.Method == http.MethodPut || r.Method == http.MethodPatch || r.Method == http.MethodDelete
		exempt := r.URL.Path == "/api/v1/auth/login" || r.URL.Path == "/api/v1/auth/signup"
		if identity != nil && mutation && !exempt && !a.auth.VerifyCSRF(r.Context(), identity.Session.ID, r.Header.Get("X-CSRF-Token")) {
			writeError(w, http.StatusForbidden, "csrf_required", "a valid X-CSRF-Token is required")
			return
		}
		next.ServeHTTP(w, r)
	})
}

func (a *application) enforceAccess(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		protected := strings.HasPrefix(r.URL.Path, "/api/v1/queue") || strings.HasPrefix(r.URL.Path, "/api/v1/playback")
		if a.options.AccessMode == "accounts_required" && protected && r.Method != http.MethodGet && identityFromContext(r.Context()) == nil {
			writeError(w, http.StatusUnauthorized, "authentication_required", "sign in to perform this action")
			return
		}
		next.ServeHTTP(w, r)
	})
}

func (a *application) usernameAvailability(w http.ResponseWriter, r *http.Request) {
	username, _, err := auth.NormalizeUsername(r.PathValue("username"))
	if err != nil {
		writeError(w, http.StatusUnprocessableEntity, "invalid_username", err.Error())
		return
	}
	_, _, err = a.auth.FindUser(r.Context(), username)
	if errors.Is(err, auth.ErrUserNotFound) {
		writeJSON(w, http.StatusOK, map[string]any{"username": username, "available": true})
		return
	}
	if err != nil {
		a.internalError(w, r, "username availability", err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"username": username, "available": false})
}

func (a *application) login(w http.ResponseWriter, r *http.Request) {
	if !a.authLimiter.Allow(clientIP(r)) {
		writeError(w, http.StatusTooManyRequests, "rate_limited", "too many authentication attempts")
		return
	}
	var request credentialsRequest
	if err := decodeJSON(w, r, &request); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_request", err.Error())
		return
	}
	username, _, err := auth.NormalizeUsername(request.Username)
	if err != nil {
		writeError(w, http.StatusUnprocessableEntity, "invalid_username", err.Error())
		return
	}
	user, hash, err := a.auth.FindUser(r.Context(), username)
	if errors.Is(err, auth.ErrUserNotFound) {
		writeJSON(w, http.StatusOK, map[string]any{"status": "account_creation_required", "username": username})
		return
	}
	if err != nil {
		a.internalError(w, r, "login", err)
		return
	}
	valid, err := auth.VerifyPassword(request.Password, hash)
	if err != nil {
		a.internalError(w, r, "verify password", err)
		return
	}
	if !valid {
		writeError(w, http.StatusUnauthorized, "invalid_credentials", "invalid username or password")
		return
	}
	issued, err := a.auth.CreateSession(r.Context(), user)
	if err != nil {
		a.internalError(w, r, "create session", err)
		return
	}
	a.issueSession(w, issued, http.StatusOK)
}

func (a *application) signup(w http.ResponseWriter, r *http.Request) {
	if !a.authLimiter.Allow(clientIP(r)) {
		writeError(w, http.StatusTooManyRequests, "rate_limited", "too many authentication attempts")
		return
	}
	var request credentialsRequest
	if err := decodeJSON(w, r, &request); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_request", err.Error())
		return
	}
	if request.Password != request.PasswordConfirmation {
		writeError(w, http.StatusUnprocessableEntity, "password_confirmation_mismatch", "password confirmation does not match")
		return
	}
	issued, err := a.auth.CreateUserAndSession(r.Context(), request.Username, request.Password)
	if errors.Is(err, auth.ErrUsernameTaken) {
		writeError(w, http.StatusConflict, "username_taken", "username is already in use")
		return
	}
	if err != nil {
		if strings.Contains(err.Error(), "username") || strings.Contains(err.Error(), "password") {
			writeError(w, http.StatusUnprocessableEntity, "invalid_account", err.Error())
			return
		}
		a.internalError(w, r, "create account", err)
		return
	}
	loggerFromContext(r.Context(), a.logger).Info("account created", "user_id", issued.Session.User.ID, "username", issued.Session.User.Username)
	a.issueSession(w, issued, http.StatusCreated)
}

func (a *application) issueSession(w http.ResponseWriter, issued auth.IssuedSession, status int) {
	expires, _ := time.Parse(time.RFC3339Nano, issued.Session.ExpiresAt)
	http.SetCookie(w, &http.Cookie{Name: sessionCookieName, Value: issued.Token, Path: "/", HttpOnly: true, Secure: a.options.SecureCookie, SameSite: http.SameSiteStrictMode, Expires: expires, MaxAge: int(time.Until(expires).Seconds())})
	http.SetCookie(w, &http.Cookie{Name: csrfCookieName, Value: issued.CSRFToken, Path: "/", Secure: a.options.SecureCookie, SameSite: http.SameSiteStrictMode, Expires: expires, MaxAge: int(time.Until(expires).Seconds())})
	writeJSON(w, status, map[string]any{"status": "authenticated", "session": issued.Session, "csrf_token": issued.CSRFToken})
}

func (a *application) currentSession(w http.ResponseWriter, r *http.Request) {
	identity := identityFromContext(r.Context())
	if identity == nil {
		writeJSON(w, http.StatusOK, map[string]bool{"authenticated": false})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"authenticated": true, "session": identity.Session})
}

func (a *application) logout(w http.ResponseWriter, r *http.Request) {
	identity := identityFromContext(r.Context())
	if identity == nil {
		writeError(w, http.StatusUnauthorized, "authentication_required", "sign in to perform this action")
		return
	}
	if err := a.auth.Revoke(r.Context(), identity.Session.User.ID, identity.Session.ID); err != nil && !errors.Is(err, auth.ErrSessionNotFound) {
		a.internalError(w, r, "logout", err)
		return
	}
	http.SetCookie(w, &http.Cookie{Name: sessionCookieName, Value: "", Path: "/", HttpOnly: true, Secure: a.options.SecureCookie, SameSite: http.SameSiteStrictMode, MaxAge: -1})
	http.SetCookie(w, &http.Cookie{Name: csrfCookieName, Value: "", Path: "/", Secure: a.options.SecureCookie, SameSite: http.SameSiteStrictMode, MaxAge: -1})
	writeJSON(w, http.StatusOK, map[string]string{"status": "logged_out"})
}

func (a *application) listSessions(w http.ResponseWriter, r *http.Request) {
	identity := identityFromContext(r.Context())
	if identity == nil {
		writeError(w, http.StatusUnauthorized, "authentication_required", "sign in to perform this action")
		return
	}
	sessions, err := a.auth.ListSessions(r.Context(), identity.Session.User.ID)
	if err != nil {
		a.internalError(w, r, "list sessions", err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"sessions": sessions, "current_session_id": identity.Session.ID})
}

func (a *application) revokeSession(w http.ResponseWriter, r *http.Request) {
	identity := identityFromContext(r.Context())
	if identity == nil {
		writeError(w, http.StatusUnauthorized, "authentication_required", "sign in to perform this action")
		return
	}
	if err := a.auth.Revoke(r.Context(), identity.Session.User.ID, r.PathValue("id")); errors.Is(err, auth.ErrSessionNotFound) {
		writeError(w, http.StatusNotFound, "session_not_found", "session was not found")
		return
	} else if err != nil {
		a.internalError(w, r, "revoke session", err)
		return
	}
	if r.PathValue("id") == identity.Session.ID {
		http.SetCookie(w, &http.Cookie{Name: sessionCookieName, Value: "", Path: "/", MaxAge: -1, HttpOnly: true})
		http.SetCookie(w, &http.Cookie{Name: csrfCookieName, Value: "", Path: "/", MaxAge: -1})
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "revoked"})
}
