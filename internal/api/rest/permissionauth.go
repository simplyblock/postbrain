package rest

import (
	"net/http"

	"github.com/simplyblock/postbrain/internal/auth"
	"github.com/simplyblock/postbrain/internal/db"
)

func (ro *Router) permissionAuthzMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		token, _ := r.Context().Value(auth.ContextKeyToken).(*db.Token)
		if token == nil {
			writeError(w, http.StatusUnauthorized, "unauthorized")
			return
		}

		if requiresWritePermission(r.Method) {
			if !auth.HasWritePermission(token.Permissions) {
				writeError(w, http.StatusForbidden, "forbidden: insufficient permissions")
				return
			}
		} else {
			if !auth.HasReadPermission(token.Permissions) {
				writeError(w, http.StatusForbidden, "forbidden: insufficient permissions")
				return
			}
		}

		next.ServeHTTP(w, r)
	})
}

func requiresWritePermission(method string) bool {
	switch method {
	case http.MethodGet, http.MethodHead, http.MethodOptions:
		return false
	default:
		return true
	}
}
