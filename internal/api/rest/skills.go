package rest

import (
	"errors"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"github.com/simplyblock/postbrain/internal/auth"
	"github.com/simplyblock/postbrain/internal/db"
	"github.com/simplyblock/postbrain/internal/db/compat"
	skillspkg "github.com/simplyblock/postbrain/internal/skills"
)

// skillFileRequest is the wire type for a supplementary skill file.
type skillFileRequest struct {
	Path       string `json:"path"`
	Content    string `json:"content"`
	Executable bool   `json:"executable"`
}

func (f skillFileRequest) toInput() db.SkillFileInput {
	return db.SkillFileInput{
		RelativePath: f.Path,
		Content:      f.Content,
		IsExecutable: f.Executable,
	}
}

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
	Files          []skillFileRequest  `json:"files"`
}

func (r *createSkillRequest) validate() error {
	if r.Scope == "" || r.Slug == "" || r.Name == "" {
		return errors.New("scope, slug and name are required")
	}
	return nil
}

func (r *createSkillRequest) applyDefaults() {
	if r.Visibility == "" {
		r.Visibility = "team"
	}
}

func (ro *Router) createSkill(w http.ResponseWriter, r *http.Request) {
	var body createSkillRequest
	if err := readJSON(r, &body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if err := body.validate(); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	if err := skillspkg.ValidateSlug(body.Slug); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	fileInputs := make([]db.SkillFileInput, len(body.Files))
	for i, f := range body.Files {
		fileInputs[i] = f.toInput()
		if err := skillspkg.ValidateSkillFile(fileInputs[i]); err != nil {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}
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
	skill, err := ro.sklStore.Create(r.Context(), skillspkg.CreateInput{
		ScopeID:        scope.ID,
		AuthorID:       authorID,
		Slug:           body.Slug,
		Name:           body.Name,
		Description:    body.Description,
		AgentTypes:     body.AgentTypes,
		Body:           body.Body,
		Parameters:     body.Parameters,
		Visibility:     body.Visibility,
		ReviewRequired: body.ReviewRequired,
		Files:          fileInputs,
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
		scope, err := compat.GetScopeByExternalID(r.Context(), ro.pool, kind, externalID)
		if err != nil || scope == nil {
			writeError(w, http.StatusBadRequest, "scope not found")
			return
		}
		if err := ro.authorizeRequestedScope(r.Context(), scope.ID); err != nil {
			writeScopeAuthzError(w, r, scope.ID, err)
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
	if err := ro.authorizeObjectScope(r.Context(), skill.ScopeID); err != nil {
		writeScopeAuthzError(w, r, skill.ScopeID, err)
		return
	}
	writeJSON(w, http.StatusOK, skill)
}

type updateSkillRequest struct {
	Body       string              `json:"body"`
	Parameters []db.SkillParameter `json:"parameters"`
	// Files: nil = leave existing files untouched; non-nil (even []) = replace all files.
	Files *[]skillFileRequest `json:"files"`
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
	var fileInputs *[]db.SkillFileInput
	if body.Files != nil {
		inputs := make([]db.SkillFileInput, len(*body.Files))
		for i, f := range *body.Files {
			inputs[i] = f.toInput()
			if err := skillspkg.ValidateSkillFile(inputs[i]); err != nil {
				writeError(w, http.StatusBadRequest, err.Error())
				return
			}
		}
		fileInputs = &inputs
	}
	existing, err := ro.sklStore.GetByID(r.Context(), id)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if existing == nil {
		writeError(w, http.StatusNotFound, "skill not found")
		return
	}
	if err := ro.authorizeObjectScope(r.Context(), existing.ScopeID); err != nil {
		writeScopeAuthzError(w, r, existing.ScopeID, err)
		return
	}
	callerID, _ := r.Context().Value(auth.ContextKeyPrincipalID).(uuid.UUID)
	updated, err := ro.sklStore.UpdateWithFiles(r.Context(), id, callerID, skillspkg.UpdateInput{
		Body:       body.Body,
		Parameters: body.Parameters,
		Files:      fileInputs,
	})
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

	skill, err := ro.sklStore.GetByID(r.Context(), id)
	if err != nil || skill == nil {
		writeError(w, http.StatusNotFound, "skill not found")
		return
	}
	files, err := compat.ListSkillFiles(r.Context(), ro.pool, skill.ID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	path, err := skillspkg.Install(skill, files, body.AgentType, ".")
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	filePaths := make([]string, len(files))
	for i, f := range files {
		filePaths[i] = f.RelativePath
	}
	writeJSON(w, http.StatusOK, map[string]any{"path": path, "slug": skill.Slug, "files": filePaths})
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

func (ro *Router) registerSkillRoutes(r chi.Router) {
	r.Post("/skills", ro.createSkill)
	r.Get("/skills/search", ro.searchSkills)
	r.Get("/skills/{id}", ro.getSkill)
	r.Patch("/skills/{id}", ro.updateSkill)
	r.Post("/skills/{id}/endorse", ro.endorseSkill)
	r.Post("/skills/{id}/deprecate", ro.deprecateSkill)
	r.Post("/skills/{id}/install", ro.installSkill)
	r.Post("/skills/{id}/invoke", ro.invokeSkill)
}
