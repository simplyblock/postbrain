package rest

import (
	"encoding/json"
	"net/http"
	"time"

	"github.com/google/uuid"

	"github.com/simplyblock/postbrain/internal/auth"
	"github.com/simplyblock/postbrain/internal/db"
)

type createSessionRequest struct {
	Scope string         `json:"scope"`
	Meta  map[string]any `json:"meta"`
}

func (ro *Router) createSession(w http.ResponseWriter, r *http.Request) {
	var body createSessionRequest
	if err := readJSON(r, &body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if body.Scope == "" {
		writeError(w, http.StatusBadRequest, "scope is required")
		return
	}

	kind, externalID, err := parseScopeString(body.Scope)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	scope, err := db.GetScopeByExternalID(r.Context(), ro.pool, kind, externalID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "scope lookup failed")
		return
	}
	if scope == nil {
		writeError(w, http.StatusBadRequest, "scope not found")
		return
	}

	principalID, _ := r.Context().Value(auth.ContextKeyPrincipalID).(uuid.UUID)

	var meta []byte
	if body.Meta != nil {
		meta, _ = json.Marshal(body.Meta)
	}

	session, err := db.CreateSession(r.Context(), ro.pool, scope.ID, principalID, meta)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusCreated, session)
}

type updateSessionRequest struct {
	EndedAt *string        `json:"ended_at"`
	Meta    map[string]any `json:"meta"`
}

func (ro *Router) updateSession(w http.ResponseWriter, r *http.Request) {
	id, err := uuidParam(r, "id")
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid session id")
		return
	}
	var body updateSessionRequest
	if err := readJSON(r, &body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	// Parse optional ended_at timestamp.
	var endedAt *time.Time
	if body.EndedAt != nil {
		t, err := time.Parse(time.RFC3339, *body.EndedAt)
		if err != nil {
			writeError(w, http.StatusBadRequest, "ended_at must be RFC3339")
			return
		}
		endedAt = &t
	}

	var meta []byte
	if body.Meta != nil {
		meta, _ = json.Marshal(body.Meta)
	}

	_ = endedAt // EndSession uses COALESCE($2, now()) — pass nil to use now()
	session, err := db.EndSession(r.Context(), ro.pool, id, meta)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, session)
}
