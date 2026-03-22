package rest

import (
	"net/http"
	"time"

	"github.com/google/uuid"

	"github.com/simplyblock/postbrain/internal/auth"
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

	var scopeID uuid.UUID
	if body.Scope != "" {
		kind, externalID, err := parseScopeString(body.Scope)
		if err != nil {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}
		// TODO(task-sessions): move to dedicated sessions store.
		_ = kind
		_ = externalID
	}

	principalID, _ := r.Context().Value(auth.ContextKeyPrincipalID).(uuid.UUID)

	sessionID := uuid.New()
	// TODO(task-sessions): persist session row to database.
	writeJSON(w, http.StatusCreated, map[string]any{
		"session_id":   sessionID,
		"scope_id":     scopeID,
		"principal_id": principalID,
		"started_at":   time.Now().UTC(),
	})
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
	// TODO(task-sessions): persist session update to database.
	writeJSON(w, http.StatusOK, map[string]any{"session_id": id, "updated": true})
}
