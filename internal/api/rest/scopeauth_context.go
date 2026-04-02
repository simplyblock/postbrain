package rest

import (
	"net/http"

	"github.com/google/uuid"

	"github.com/simplyblock/postbrain/internal/api/scopeauth"
	"github.com/simplyblock/postbrain/internal/auth"
)

func (ro *Router) scopeAuthzContextMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if ro.membership == nil {
			next.ServeHTTP(w, r)
			return
		}

		principalID, _ := r.Context().Value(auth.ContextKeyPrincipalID).(uuid.UUID)
		if principalID == uuid.Nil {
			next.ServeHTTP(w, r)
			return
		}

		effectiveScopeIDs, err := ro.membership.EffectiveScopeIDs(r.Context(), principalID)
		if err != nil {
			writeError(w, http.StatusInternalServerError, "scope authorization failed")
			return
		}

		ctx := scopeauth.WithEffectiveScopeIDs(r.Context(), effectiveScopeIDs)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}
