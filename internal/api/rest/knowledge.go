package rest

import (
	"errors"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"github.com/simplyblock/postbrain/internal/auth"
	"github.com/simplyblock/postbrain/internal/db/compat"
	"github.com/simplyblock/postbrain/internal/knowledge"
)

type createArtifactRequest struct {
	Title          string  `json:"title"`
	Content        string  `json:"content"`
	KnowledgeType  string  `json:"knowledge_type"`
	ArtifactKind   string  `json:"artifact_kind"`
	Scope          string  `json:"scope"`
	Visibility     string  `json:"visibility"`
	Summary        *string `json:"summary"`
	AutoReview     bool    `json:"auto_review"`
	CollectionSlug string  `json:"collection_slug"`
}

func (r *createArtifactRequest) validate() error {
	if r.Title == "" || r.Content == "" || r.KnowledgeType == "" || r.Scope == "" {
		return errors.New("title, content, knowledge_type and scope are required")
	}
	return nil
}

func (r *createArtifactRequest) applyDefaults() {
	if r.Visibility == "" {
		r.Visibility = "team"
	}
}

func (ro *Router) createArtifact(w http.ResponseWriter, r *http.Request) {
	var body createArtifactRequest
	if err := readJSON(r, &body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if err := body.validate(); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	body.applyDefaults()

	kind, externalID, err := parseScopeString(body.Scope)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	scope, err := compat.GetScopeByExternalID(r.Context(), ro.pool, kind, externalID)
	if err != nil || scope == nil {
		writeError(w, http.StatusBadRequest, "scope not found")
		return
	}
	if err := ro.authorizeRequestedScope(r.Context(), scope.ID); err != nil {
		writeScopeAuthzError(w, r, scope.ID, err)
		return
	}

	authorID, _ := r.Context().Value(auth.ContextKeyPrincipalID).(uuid.UUID)
	artifactKind, err := knowledge.NormalizeArtifactKind(body.ArtifactKind)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	artifact, err := ro.knwStore.Create(r.Context(), knowledge.CreateInput{
		KnowledgeType: body.KnowledgeType,
		ArtifactKind:  artifactKind,
		OwnerScopeID:  scope.ID,
		AuthorID:      authorID,
		Visibility:    body.Visibility,
		Title:         body.Title,
		Content:       body.Content,
		Summary:       body.Summary,
		AutoReview:    body.AutoReview,
	})
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	if body.CollectionSlug != "" && ro.knwColl != nil {
		coll, err := ro.knwColl.GetBySlug(r.Context(), scope.ID, body.CollectionSlug)
		if err == nil && coll != nil {
			_ = ro.knwColl.AddItem(r.Context(), coll.ID, artifact.ID, authorID)
		}
	}

	writeJSON(w, http.StatusCreated, artifact)
}

func (ro *Router) searchArtifacts(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	query := q.Get("q")
	scopeStr := q.Get("scope")
	pg := paginationFromRequest(r)

	var scopeID uuid.UUID
	if scopeStr != "" {
		kind, externalID, err := parseScopeString(scopeStr)
		if err != nil {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}
		scope, err := compat.GetScopeByExternalID(r.Context(), ro.pool, kind, externalID)
		if err != nil || scope == nil {
			writeError(w, http.StatusBadRequest, "scope not found")
			return
		}
		if err := ro.authorizeRequestedScope(r.Context(), scope.ID); err != nil {
			writeScopeAuthzError(w, r, scope.ID, err)
			return
		}
		scopeID = scope.ID
	}

	results, err := ro.knwStore.Recall(r.Context(), ro.pool, knowledge.RecallInput{
		Query:   query,
		ScopeID: scopeID,
		Limit:   pg.Limit,
	})
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"results": results})
}

func (ro *Router) getArtifact(w http.ResponseWriter, r *http.Request) {
	id, err := uuidParam(r, "id")
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid artifact id")
		return
	}
	a, err := ro.knwStore.GetByID(r.Context(), id)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if a == nil {
		writeError(w, http.StatusNotFound, "artifact not found")
		return
	}
	if err := ro.authorizeObjectScope(r.Context(), a.OwnerScopeID); err != nil {
		writeScopeAuthzError(w, r, a.OwnerScopeID, err)
		return
	}
	writeJSON(w, http.StatusOK, a)
}

type updateArtifactRequest struct {
	Title   string  `json:"title"`
	Content string  `json:"content"`
	Summary *string `json:"summary"`
}

func (ro *Router) updateArtifact(w http.ResponseWriter, r *http.Request) {
	id, err := uuidParam(r, "id")
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid artifact id")
		return
	}
	var body updateArtifactRequest
	if err := readJSON(r, &body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	existing, err := ro.knwStore.GetByID(r.Context(), id)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if existing == nil {
		writeError(w, http.StatusNotFound, "artifact not found")
		return
	}
	if err := ro.authorizeObjectScope(r.Context(), existing.OwnerScopeID); err != nil {
		writeScopeAuthzError(w, r, existing.OwnerScopeID, err)
		return
	}
	callerID, _ := r.Context().Value(auth.ContextKeyPrincipalID).(uuid.UUID)
	updated, err := ro.knwStore.Update(r.Context(), id, callerID, body.Title, body.Content, body.Summary)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, updated)
}

type endorseArtifactRequest struct {
	Note *string `json:"note"`
}

func (ro *Router) endorseArtifact(w http.ResponseWriter, r *http.Request) {
	id, err := uuidParam(r, "id")
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid artifact id")
		return
	}
	var body endorseArtifactRequest
	_ = readJSON(r, &body)

	endorserID, _ := r.Context().Value(auth.ContextKeyPrincipalID).(uuid.UUID)
	result, err := ro.knwLife.Endorse(r.Context(), id, endorserID, body.Note)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, result)
}

func (ro *Router) deprecateArtifact(w http.ResponseWriter, r *http.Request) {
	id, err := uuidParam(r, "id")
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid artifact id")
		return
	}
	callerID, _ := r.Context().Value(auth.ContextKeyPrincipalID).(uuid.UUID)
	if err := ro.knwLife.Deprecate(r.Context(), id, callerID); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"artifact_id": id, "status": "deprecated"})
}

func (ro *Router) deleteArtifact(w http.ResponseWriter, r *http.Request) {
	id, err := uuidParam(r, "id")
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid artifact id")
		return
	}
	callerID, _ := r.Context().Value(auth.ContextKeyPrincipalID).(uuid.UUID)
	if err := ro.knwLife.Delete(r.Context(), id, callerID); err != nil {
		switch err {
		case knowledge.ErrForbidden:
			writeError(w, http.StatusForbidden, "caller is not a scope admin")
		case knowledge.ErrInvalidTransition:
			writeError(w, http.StatusNotFound, "artifact not found")
		default:
			writeError(w, http.StatusInternalServerError, err.Error())
		}
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (ro *Router) getArtifactHistory(w http.ResponseWriter, r *http.Request) {
	id, err := uuidParam(r, "id")
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid artifact id")
		return
	}
	history, err := compat.GetArtifactHistory(r.Context(), ro.pool, id)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"artifact_id": id, "history": history})
}

func (ro *Router) registerKnowledgeRoutes(r chi.Router) {
	r.Post("/knowledge/upload", ro.uploadKnowledge)
	r.Post("/knowledge/synthesize", ro.synthesizeKnowledge)
	r.Post("/knowledge", ro.createArtifact)
	r.Get("/knowledge/search", ro.searchArtifacts)
	r.Get("/knowledge/{id}", ro.getArtifact)
	r.Patch("/knowledge/{id}", ro.updateArtifact)
	r.Delete("/knowledge/{id}", ro.deleteArtifact)
	r.Post("/knowledge/{id}/endorse", ro.endorseArtifact)
	r.Post("/knowledge/{id}/deprecate", ro.deprecateArtifact)
	r.Get("/knowledge/{id}/history", ro.getArtifactHistory)
	r.Get("/knowledge/{id}/sources", ro.getArtifactSources)
	r.Get("/knowledge/{id}/digests", ro.getArtifactDigests)
}
