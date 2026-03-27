package rest

import (
	"net/http"

	"github.com/google/uuid"

	"github.com/simplyblock/postbrain/internal/auth"
	"github.com/simplyblock/postbrain/internal/db"
	"github.com/simplyblock/postbrain/internal/knowledge"
)

type createArtifactRequest struct {
	Title          string  `json:"title"`
	Content        string  `json:"content"`
	KnowledgeType  string  `json:"knowledge_type"`
	Scope          string  `json:"scope"`
	Visibility     string  `json:"visibility"`
	Summary        *string `json:"summary"`
	AutoReview     bool    `json:"auto_review"`
	CollectionSlug string  `json:"collection_slug"`
}

func (ro *Router) createArtifact(w http.ResponseWriter, r *http.Request) {
	var body createArtifactRequest
	if err := readJSON(r, &body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if body.Title == "" || body.Content == "" || body.KnowledgeType == "" || body.Scope == "" {
		writeError(w, http.StatusBadRequest, "title, content, knowledge_type and scope are required")
		return
	}
	visibility := body.Visibility
	if visibility == "" {
		visibility = "team"
	}

	kind, externalID, err := parseScopeString(body.Scope)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	scope, err := db.GetScopeByExternalID(r.Context(), ro.pool, kind, externalID)
	if err != nil || scope == nil {
		writeError(w, http.StatusBadRequest, "scope not found")
		return
	}

	authorID, _ := r.Context().Value(auth.ContextKeyPrincipalID).(uuid.UUID)

	artifact, err := ro.knwStore.Create(r.Context(), knowledge.CreateInput{
		KnowledgeType: body.KnowledgeType,
		OwnerScopeID:  scope.ID,
		AuthorID:      authorID,
		Visibility:    visibility,
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
		scope, err := db.GetScopeByExternalID(r.Context(), ro.pool, kind, externalID)
		if err != nil || scope == nil {
			writeError(w, http.StatusBadRequest, "scope not found")
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

func (ro *Router) getArtifactHistory(w http.ResponseWriter, r *http.Request) {
	id, err := uuidParam(r, "id")
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid artifact id")
		return
	}
	history, err := db.GetArtifactHistory(r.Context(), ro.pool, id)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"artifact_id": id, "history": history})
}
