package rest

import (
	"context"
	"log/slog"
	"net/http"

	"github.com/go-chi/chi/v5/middleware"

	"github.com/simplyblock/postbrain/internal/auth"
)

type loggerKey struct{}

// requestLoggerMiddleware injects a slog.Logger enriched with request_id and
// principal_id (when available) into the request context.
func requestLoggerMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		reqID := middleware.GetReqID(r.Context())
		logger := slog.Default().With("request_id", reqID)

		// Enrich with principal_id if auth middleware has already run.
		if pid := r.Context().Value(auth.ContextKeyPrincipalID); pid != nil {
			logger = logger.With("principal_id", pid)
		}

		ctx := context.WithValue(r.Context(), loggerKey{}, logger)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

// LogFromContext returns the slog.Logger stored in ctx by requestLoggerMiddleware,
// or slog.Default() if none is found.
func LogFromContext(ctx context.Context) *slog.Logger {
	if l, ok := ctx.Value(loggerKey{}).(*slog.Logger); ok && l != nil {
		return l
	}
	return slog.Default()
}
