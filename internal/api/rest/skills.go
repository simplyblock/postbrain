package rest

import (
	"net/http"

	"github.com/google/uuid"

	"github.com/simplyblock/postbrain/internal/auth"
	"github.com/simplyblock/postbrain/internal/db"
	skillspkg "github.com/simplyblock/postbrain/internal/skills"
)

type createSkillRequest struct {
	Scope          string              `json:"scope"`
	Slug           string              `json:"slug"`
	Name           string              `json:"name"`
	Description    string              `json:"description"`
	AgentTypes     []string            `json:"agent_types"`
	Body           string              `json:"body"`
	Parameters     []db.SkillParameter `json:"parameters"`
	Visibility     string              `json:"visibility"`
	ReviewRequired int                 `json:"review_required"`
}

func (ro *Router) createSkill(w http.ResponseWriter, r *http.Request) {
	var body createSkillRequest
	if err := readJSON(r, &body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if body.Scope == "" || body.Slug == "" || body.Name == "" {
		writeError(w, http.StatusBadRequest, "scope, slug and name are required")
		return
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

	visibility := body.Visibility
	if visibility == "" {
		visibility = "team"
	}

	authorID, _ := r.Context().Value(auth.ContextKeyPrincipalID).(uuid.UUID)
	skill, err := ro.sklStore.Create(r.Context(), skillspkg.CreateInput{
		ScopeID:        scope.ID,
		AuthorID:       authorID,
		Slug:           body.Slug,
		Name:           body.Name,
		Description:    body.Description,
		AgentTypes:     body.AgentTypes,
		Body:           body.Body,
		Parameters:     body.Parameters,
		Visibility:     visibility,
		ReviewRequired: body.ReviewRequired,
	})
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusCreated, skill)
}

func (ro *Router) searchSkills(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	query := q.Get("q")
	scopeStr := q.Get("scope")
	agentType := q.Get("agent_type")
	pg := paginationFromRequest(r)

	var scopeIDs []uuid.UUID
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
		scopeIDs = []uuid.UUID{scope.ID}
	}

	if ro.svc == nil {
		writeError(w, http.StatusServiceUnavailable, "embedding service not configured")
		return
	}

	results, err := ro.sklStore.Recall(r.Context(), ro.svc, skillspkg.RecallInput{
		Query:     query,
		ScopeIDs:  scopeIDs,
		AgentType: agentType,
		Limit:     pg.Limit,
	})
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"results": results})
}

func (ro *Router) getSkill(w http.ResponseWriter, r *http.Request) {
	id, err := uuidParam(r, "id")
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid skill id")
		return
	}
	skill, err := ro.sklStore.GetByID(r.Context(), id)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if skill == nil {
		writeError(w, http.StatusNotFound, "skill not found")
		return
	}
	writeJSON(w, http.StatusOK, skill)
}

type updateSkillRequest struct {
	Body       string              `json:"body"`
	Parameters []db.SkillParameter `json:"parameters"`
}

func (ro *Router) updateSkill(w http.ResponseWriter, r *http.Request) {
	id, err := uuidParam(r, "id")
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid skill id")
		return
	}
	var body updateSkillRequest
	if err := readJSON(r, &body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	callerID, _ := r.Context().Value(auth.ContextKeyPrincipalID).(uuid.UUID)
	updated, err := ro.sklStore.Update(r.Context(), id, callerID, body.Body, body.Parameters)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, updated)
}

type endorseSkillRequest struct {
	Note *string `json:"note"`
}

func (ro *Router) endorseSkill(w http.ResponseWriter, r *http.Request) {
	id, err := uuidParam(r, "id")
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid skill id")
		return
	}
	var body endorseSkillRequest
	_ = readJSON(r, &body)

	endorserID, _ := r.Context().Value(auth.ContextKeyPrincipalID).(uuid.UUID)
	result, err := ro.sklLife.Endorse(r.Context(), id, endorserID, body.Note)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, result)
}

func (ro *Router) deprecateSkill(w http.ResponseWriter, r *http.Request) {
	id, err := uuidParam(r, "id")
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid skill id")
		return
	}
	callerID, _ := r.Context().Value(auth.ContextKeyPrincipalID).(uuid.UUID)
	if err := ro.sklLife.Deprecate(r.Context(), id, callerID); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"skill_id": id, "status": "deprecated"})
}

type installSkillRequest struct {
	AgentType string `json:"agent_type"`
	Workdir   string `json:"workdir"`
}

func (ro *Router) installSkill(w http.ResponseWriter, r *http.Request) {
	id, err := uuidParam(r, "id")
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid skill id")
		return
	}
	var body installSkillRequest
	_ = readJSON(r, &body)
	if body.AgentType == "" {
		body.AgentType = "claude-code"
	}
	if body.Workdir == "" {
		body.Workdir = "."
	}

	skill, err := ro.sklStore.GetByID(r.Context(), id)
	if err != nil || skill == nil {
		writeError(w, http.StatusNotFound, "skill not found")
		return
	}
	path, err := skillspkg.Install(skill, body.AgentType, body.Workdir)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"path": path, "slug": skill.Slug})
}

type invokeSkillRequest struct {
	Params map[string]any `json:"params"`
}

func (ro *Router) invokeSkill(w http.ResponseWriter, r *http.Request) {
	id, err := uuidParam(r, "id")
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid skill id")
		return
	}
	var body invokeSkillRequest
	if err := readJSON(r, &body); err != nil {
		body.Params = map[string]any{}
	}

	skill, err := ro.sklStore.GetByID(r.Context(), id)
	if err != nil || skill == nil {
		writeError(w, http.StatusNotFound, "skill not found")
		return
	}
	result, err := skillspkg.Invoke(skill, body.Params)
	if err != nil {
		if _, ok := err.(*skillspkg.ValidationError); ok {
			writeError(w, http.StatusUnprocessableEntity, err.Error())
			return
		}
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"skill_id": skill.ID,
		"slug":     skill.Slug,
		"body":     result,
	})
}
