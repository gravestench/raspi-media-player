package app

import (
	"context"
	"database/sql"
	"embed"
	"encoding/json"
	"io/fs"
	"log/slog"
	"net/http"
	"strings"
	"time"
)

//go:embed web/*
var webFiles embed.FS

type BuildInfo struct {
	Version string `json:"version"`
	Commit  string `json:"commit"`
	BuiltAt string `json:"built_at"`
}

type application struct {
	logger *slog.Logger
	db     *sql.DB
	build  BuildInfo
}

type requestLoggerKey struct{}

func New(logger *slog.Logger, db *sql.DB, build BuildInfo) http.Handler {
	a := &application{logger: logger, db: db, build: build}
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/v1/health/live", a.live)
	mux.HandleFunc("GET /api/v1/health/ready", a.ready)
	mux.HandleFunc("GET /api/v1/version", a.version)
	static, _ := fs.Sub(webFiles, "web")
	mux.Handle("GET /", http.FileServerFS(static))
	return requestLogging(logger, mux)
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
		r = r.WithContext(context.WithValue(r.Context(), requestLoggerKey{}, requestLogger))
		recorder := &responseRecorder{ResponseWriter: w}
		next.ServeHTTP(recorder, r)
		requestLogger.Info("http request",
			"method", r.Method,
			"path", r.URL.Path,
			"route", r.Pattern,
			"status", recorder.status,
			"duration_ms", time.Since(started).Milliseconds(),
			"response_bytes", recorder.size,
			"remote_address", r.RemoteAddr,
		)
	})
}

func loggerFromContext(ctx context.Context, fallback *slog.Logger) *slog.Logger {
	if logger, ok := ctx.Value(requestLoggerKey{}).(*slog.Logger); ok {
		return logger
	}
	return fallback
}
