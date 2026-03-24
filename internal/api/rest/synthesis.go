package rest

import (
	"net/http"

	"github.com/google/uuid"

	"github.com/simplyblock/postbrain/internal/auth"
	"github.com/simplyblock/postbrain/internal/db"
	"github.com/simplyblock/postbrain/internal/knowledge"
)

type synthesizeRequest struct {
	ScopeID    string   `json:"scope_id"`
	SourceIDs  []string `json:"source_ids"`
	Title      string   `json:"title"`
	Visibility string   `json:"visibility"`
	AutoReview bool     `json:"auto_review"`
}

func (ro *Router) synthesizeKnowledge(w http.ResponseWriter, r *http.Request) {
	var body synthesizeRequest
	if err := readJSON(r, &body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	scopeID, err := uuid.Parse(body.ScopeID)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid scope_id")
		return
	}

	if len(body.SourceIDs) < 2 {
		writeError(w, http.StatusBadRequest, "source_ids must contain at least 2 artifact IDs")
		return
	}

	sourceIDs := make([]uuid.UUID, 0, len(body.SourceIDs))
	for _, s := range body.SourceIDs {
		id, err := uuid.Parse(s)
		if err != nil {
			writeError(w, http.StatusBadRequest, "invalid source_id: "+s)
			return
		}
		sourceIDs = append(sourceIDs, id)
	}

	authorID, _ := r.Context().Value(auth.ContextKeyPrincipalID).(uuid.UUID)

	synth := knowledge.NewSynthesiser(ro.pool, ro.svc)
	artifact, err := synth.Create(r.Context(), knowledge.SynthesisInput{
		ScopeID:    scopeID,
		AuthorID:   authorID,
		SourceIDs:  sourceIDs,
		Title:      body.Title,
		Visibility: body.Visibility,
		AutoReview: body.AutoReview,
	})
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	writeJSON(w, http.StatusCreated, artifact)
}

func (ro *Router) getArtifactSources(w http.ResponseWriter, r *http.Request) {
	id, err := uuidParam(r, "id")
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid artifact id")
		return
	}

	synth := knowledge.NewSynthesiser(ro.pool, ro.svc)
	sources, err := synth.ListSources(r.Context(), id)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"artifact_id": id, "sources": sources})
}

func (ro *Router) getArtifactDigests(w http.ResponseWriter, r *http.Request) {
	id, err := uuidParam(r, "id")
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid artifact id")
		return
	}

	// Validate artifact exists.
	a, err := db.GetArtifact(r.Context(), ro.pool, id)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if a == nil {
		writeError(w, http.StatusNotFound, "artifact not found")
		return
	}

	synth := knowledge.NewSynthesiser(ro.pool, ro.svc)
	digests, err := synth.ListDigests(r.Context(), id)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"artifact_id": id, "digests": digests})
}
