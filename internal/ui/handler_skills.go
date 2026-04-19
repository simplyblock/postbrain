package ui

import (
	"net/http"
	"strings"

	"github.com/google/uuid"

	"github.com/simplyblock/postbrain/internal/db"
	"github.com/simplyblock/postbrain/internal/db/compat"
)

// handleSkills serves GET /ui/{scope}/skills.
func (h *Handler) handleSkills(w http.ResponseWriter, r *http.Request) {
	scope := scopeFromContext(r.Context())

	data := struct {
		ScopeID string
		Skills  []*db.Skill
	}{}
	if scope != nil {
		data.ScopeID = scope.ID.String()
	}

	if h.pool != nil && scope != nil {
		_, scopeSet := h.authorizedScopesForRequest(r.Context(), r)
		skills, err := compat.ListPublishedSkillsForAgent(r.Context(), h.pool, []uuid.UUID{scope.ID}, "any")
		if err == nil {
			filtered := make([]*db.Skill, 0, len(skills))
			for _, s := range skills {
				if _, ok := scopeSet[s.ScopeID]; ok {
					filtered = append(filtered, s)
				}
			}
			data.Skills = filtered
		}
	}

	h.render(w, r, "skills", "Skills", data)
}

// handleSkillDetail serves GET /ui/{scope}/skills/{id}.
func (h *Handler) handleSkillDetail(w http.ResponseWriter, r *http.Request) {
	path := routePathFromContext(r.Context(), r)
	idStr := strings.TrimPrefix(path, "/ui/skills/")
	id, err := uuid.Parse(idStr)
	if err != nil {
		http.NotFound(w, r)
		return
	}

	scopeID := ""
	if scope := scopeFromContext(r.Context()); scope != nil {
		scopeID = scope.ID.String()
	}

	data := struct {
		Skill   *db.Skill
		ScopeID string
	}{ScopeID: scopeID}

	if h.pool != nil {
		skill, err := compat.GetSkill(r.Context(), h.pool, id)
		if err != nil || skill == nil {
			http.NotFound(w, r)
			return
		}
		data.Skill = skill
	}

	h.render(w, r, "skill_detail", "Skill", data)
}

// handleSkillHistory serves GET /ui/{scope}/skills/{id}/history.
func (h *Handler) handleSkillHistory(w http.ResponseWriter, r *http.Request) {
	path := routePathFromContext(r.Context(), r)
	idStr := strings.TrimSuffix(strings.TrimPrefix(path, "/ui/skills/"), "/history")
	id, err := uuid.Parse(idStr)
	if err != nil {
		http.NotFound(w, r)
		return
	}

	scopeID := ""
	if scope := scopeFromContext(r.Context()); scope != nil {
		scopeID = scope.ID.String()
	}

	data := struct {
		Skill   *db.Skill
		History []*db.SkillHistory
		ScopeID string
	}{ScopeID: scopeID}

	if h.pool != nil {
		skill, err := compat.GetSkill(r.Context(), h.pool, id)
		if err != nil || skill == nil {
			http.NotFound(w, r)
			return
		}
		data.Skill = skill
		history, _ := compat.GetSkillHistory(r.Context(), h.pool, id)
		data.History = history
	}

	h.render(w, r, "skill_history", "Skill History", data)
}
