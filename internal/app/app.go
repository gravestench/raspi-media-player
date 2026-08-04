package app

import (
	"context"
	"database/sql"
	"embed"
	"encoding/json"
	"fmt"
	"io/fs"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/dylanknuth/raspi-media-player/internal/auth"
	"github.com/dylanknuth/raspi-media-player/internal/playback"
	queuepkg "github.com/dylanknuth/raspi-media-player/internal/queue"
)

//go:embed web/*
var webFiles embed.FS

type BuildInfo struct {
	Version string `json:"version"`
	Commit  string `json:"commit"`
	BuiltAt string `json:"built_at"`
}

type application struct {
	logger      *slog.Logger
	db          *sql.DB
	build       BuildInfo
	queue       *queuepkg.Store
	options     Options
	limiter     *rateLimiter
	auth        *auth.Store
	authLimiter *rateLimiter
	playback    *playback.Controller
}

type Options struct {
	QueueLimit      int
	QueueRate       int
	AccessMode      string
	AuthRate        int
	SessionLifetime time.Duration
	SecureCookie    bool
	ArgonMemory     uint32
	ArgonIterations uint32
	Playback        *playback.Controller
}

type requestLoggerKey struct{}
type requestMetadataKey struct{}
type requestMetadata struct {
	userID   string
	username string
	route    string
}

func New(logger *slog.Logger, db *sql.DB, build BuildInfo, options ...Options) (http.Handler, error) {
	opts := Options{QueueLimit: 100, QueueRate: 20, AccessMode: "open", AuthRate: 10, SessionLifetime: 30 * 24 * time.Hour, ArgonMemory: 64 * 1024, ArgonIterations: 3}
	if len(options) > 0 {
		provided := options[0]
		if provided.QueueLimit > 0 {
			opts.QueueLimit = provided.QueueLimit
		}
		if provided.QueueRate > 0 {
			opts.QueueRate = provided.QueueRate
		}
		if provided.AccessMode != "" {
			opts.AccessMode = provided.AccessMode
		}
		if provided.AuthRate > 0 {
			opts.AuthRate = provided.AuthRate
		}
		if provided.SessionLifetime > 0 {
			opts.SessionLifetime = provided.SessionLifetime
		}
		if provided.ArgonMemory > 0 {
			opts.ArgonMemory = provided.ArgonMemory
		}
		if provided.ArgonIterations > 0 {
			opts.ArgonIterations = provided.ArgonIterations
		}
		opts.SecureCookie = provided.SecureCookie
		opts.Playback = provided.Playback
	}
	if opts.AccessMode != "open" && opts.AccessMode != "accounts_optional" && opts.AccessMode != "accounts_required" {
		return nil, fmt.Errorf("invalid access mode %q", opts.AccessMode)
	}
	params := auth.DefaultPasswordParams()
	params.Memory = opts.ArgonMemory
	params.Iterations = opts.ArgonIterations
	a := &application{logger: logger, db: db, build: build, queue: queuepkg.NewStore(db), options: opts, limiter: newRateLimiter(opts.QueueRate, time.Minute), authLimiter: newRateLimiter(opts.AuthRate, time.Minute), auth: auth.NewStore(db, params, opts.SessionLifetime), playback: opts.Playback}
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/v1/health/live", a.live)
	mux.HandleFunc("GET /api/v1/health/ready", a.ready)
	mux.HandleFunc("GET /api/v1/version", a.version)
	mux.HandleFunc("GET /api/v1/queue", a.getQueue)
	mux.HandleFunc("POST /api/v1/queue/items", a.addQueueItem)
	mux.HandleFunc("DELETE /api/v1/queue/items/{id}", a.removeQueueItem)
	mux.HandleFunc("PUT /api/v1/queue/order", a.reorderQueue)
	mux.HandleFunc("DELETE /api/v1/queue", a.clearQueue)
	mux.HandleFunc("POST /api/v1/queue/skip", a.skipQueueItem)
	mux.HandleFunc("GET /api/v1/auth/usernames/{username}", a.usernameAvailability)
	mux.HandleFunc("POST /api/v1/auth/login", a.login)
	mux.HandleFunc("POST /api/v1/auth/signup", a.signup)
	mux.HandleFunc("GET /api/v1/auth/session", a.currentSession)
	mux.HandleFunc("POST /api/v1/auth/logout", a.logout)
	mux.HandleFunc("GET /api/v1/auth/sessions", a.listSessions)
	mux.HandleFunc("DELETE /api/v1/auth/sessions/{id}", a.revokeSession)
	mux.HandleFunc("POST /api/v1/playback/pause", a.pausePlayback)
	mux.HandleFunc("POST /api/v1/playback/resume", a.resumePlayback)
	mux.HandleFunc("POST /api/v1/playback/stop", a.stopPlayback)
	mux.HandleFunc("POST /api/v1/playback/seek", a.seekPlayback)
	mux.HandleFunc("PUT /api/v1/playback/volume", a.setPlaybackVolume)
	mux.HandleFunc("GET /api/v1/events", a.events)
	static, _ := fs.Sub(webFiles, "web")
	mux.Handle("GET /", http.FileServerFS(static))
	return requestLogging(logger, a.authenticate(a.protectMutations(a.enforceAccess(captureRoute(mux))))), nil
}

func (a *application) live(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func (a *application) ready(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := contextWithTimeout(r, 2*time.Second)
	defer cancel()
	if err := a.db.PingContext(ctx); err != nil {
		loggerFromContext(ctx, a.logger).ErrorContext(ctx, "readiness check failed", "error", err)
		writeError(w, http.StatusServiceUnavailable, "not_ready", "service is not ready")
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "ready"})
}

func (a *application) version(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, a.build)
}

func writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}

func writeError(w http.ResponseWriter, status int, code, message string) {
	writeJSON(w, status, map[string]any{"error": map[string]string{"code": code, "message": message}})
}

func contextWithTimeout(r *http.Request, duration time.Duration) (context.Context, context.CancelFunc) {
	return context.WithTimeout(r.Context(), duration)
}

type responseRecorder struct {
	http.ResponseWriter
	status int
	size   int
}

func (r *responseRecorder) Flush() {
	if r.status == 0 {
		r.WriteHeader(http.StatusOK)
	}
	if flusher, ok := r.ResponseWriter.(http.Flusher); ok {
		flusher.Flush()
	}
}

func (r *responseRecorder) Unwrap() http.ResponseWriter { return r.ResponseWriter }

func (r *responseRecorder) WriteHeader(status int) {
	r.status = status
	r.ResponseWriter.WriteHeader(status)
}

func (r *responseRecorder) Write(body []byte) (int, error) {
	if r.status == 0 {
		r.WriteHeader(http.StatusOK)
	}
	n, err := r.ResponseWriter.Write(body)
	r.size += n
	return n, err
}

func requestLogging(logger *slog.Logger, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		started := time.Now()
		requestID := strings.TrimSpace(r.Header.Get("X-Request-ID"))
		if requestID == "" || len(requestID) > 128 {
			requestID = newRequestID()
		}
		w.Header().Set("X-Request-ID", requestID)
		requestLogger := logger.With("request_id", requestID)
		metadata := &requestMetadata{}
		ctx := context.WithValue(r.Context(), requestLoggerKey{}, requestLogger)
		ctx = context.WithValue(ctx, requestMetadataKey{}, metadata)
		r = r.WithContext(ctx)
		recorder := &responseRecorder{ResponseWriter: w}
		next.ServeHTTP(recorder, r)
		attributes := []any{
			"method", r.Method,
			"path", r.URL.Path,
			"route", metadata.route,
			"status", recorder.status,
			"duration_ms", time.Since(started).Milliseconds(),
			"response_bytes", recorder.size,
			"remote_address", r.RemoteAddr,
		}
		if metadata.userID != "" {
			attributes = append(attributes, "user_id", metadata.userID, "username", metadata.username)
		}
		requestLogger.Info("http request", attributes...)
	})
}

func captureRoute(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		next.ServeHTTP(w, r)
		if metadata, ok := r.Context().Value(requestMetadataKey{}).(*requestMetadata); ok {
			metadata.route = r.Pattern
		}
	})
}

func loggerFromContext(ctx context.Context, fallback *slog.Logger) *slog.Logger {
	if logger, ok := ctx.Value(requestLoggerKey{}).(*slog.Logger); ok {
		return logger
	}
	return fallback
}
