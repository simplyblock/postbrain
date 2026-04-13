package ui

import (
	"net/http"
	"strings"

	"github.com/google/uuid"

	"github.com/simplyblock/postbrain/internal/db"
)

// handleSkills serves GET /ui/skills.
func (h *Handler) handleSkills(w http.ResponseWriter, r *http.Request) {
	data := struct {
		Skills []*db.Skill
	}{}

	if h.pool != nil {
		scopes, scopeSet := h.authorizedScopesForRequest(r.Context(), r)
		authorizedScopeIDs := make([]uuid.UUID, 0, len(scopes))
		for _, s := range scopes {
			authorizedScopeIDs = append(authorizedScopeIDs, s.ID)
		}
		skills, err := db.ListPublishedSkillsForAgent(r.Context(), h.pool, authorizedScopeIDs, "any")
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

// handleSkillDetail serves GET /ui/skills/{id}.
func (h *Handler) handleSkillDetail(w http.ResponseWriter, r *http.Request) {
	idStr := strings.TrimPrefix(r.URL.Path, "/ui/skills/")
	id, err := uuid.Parse(idStr)
	if err != nil {
		http.NotFound(w, r)
		return
	}

	data := struct {
		Skill *db.Skill
	}{}

	if h.pool != nil {
		skill, err := db.GetSkill(r.Context(), h.pool, id)
		if err != nil || skill == nil {
			http.NotFound(w, r)
			return
		}
		data.Skill = skill
	}

	h.render(w, r, "skill_detail", "Skill", data)
}

// handleSkillHistory serves GET /ui/skills/{id}/history.
func (h *Handler) handleSkillHistory(w http.ResponseWriter, r *http.Request) {
	idStr := strings.TrimSuffix(strings.TrimPrefix(r.URL.Path, "/ui/skills/"), "/history")
	id, err := uuid.Parse(idStr)
	if err != nil {
		http.NotFound(w, r)
		return
	}

	data := struct {
		Skill   *db.Skill
		History []*db.SkillHistory
	}{}

	if h.pool != nil {
		skill, err := db.GetSkill(r.Context(), h.pool, id)
		if err != nil || skill == nil {
			http.NotFound(w, r)
			return
		}
		data.Skill = skill
		history, _ := db.GetSkillHistory(r.Context(), h.pool, id)
		data.History = history
	}

	h.render(w, r, "skill_history", "Skill History", data)
}
