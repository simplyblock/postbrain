package ui

import (
	"net/http"
	"strconv"
	"strings"

	"github.com/google/uuid"

	"github.com/simplyblock/postbrain/internal/db"
	"github.com/simplyblock/postbrain/internal/db/compat"
)

const memoriesPageSize = 50

// handleMemories serves GET /ui/memories.
func (h *Handler) handleMemories(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query().Get("q")
	scopeID := r.URL.Query().Get("scope_id")
	offset := 0
	if c := r.URL.Query().Get("cursor"); c != "" {
		if v, err := strconv.Atoi(c); err == nil && v > 0 {
			offset = v
		}
	}

	data := struct {
		Query      string
		ScopeID    string
		Scopes     []*db.Scope
		Memories   []*db.Memory
		NextCursor string
	}{
		Query:   q,
		ScopeID: scopeID,
	}

	if h.pool != nil {
		scopes, scopeSet := h.authorizedScopesForRequest(r.Context(), r)
		data.Scopes = scopes
		if scopeID == "" && len(data.Scopes) > 0 {
			scopeID = data.Scopes[0].ID.String()
			data.ScopeID = scopeID
		}
		if scopeID != "" {
			if sid, err := uuid.Parse(scopeID); err == nil {
				if _, ok := scopeSet[sid]; !ok {
					goto doneMemories
				}
				mems, err := compat.ListMemoriesByScope(r.Context(), h.pool, sid, memoriesPageSize+1, offset)
				if err == nil {
					if len(mems) > memoriesPageSize {
						data.Memories = mems[:memoriesPageSize]
						data.NextCursor = strconv.Itoa(offset + memoriesPageSize)
					} else {
						data.Memories = mems
					}
				}
			}
		}
	}
doneMemories:

	tmpl := "memories"
	if r.Header.Get("HX-Request") == "true" {
		tmpl = "memories_rows"
	}
	h.render(w, r, tmpl, "Memories", data)
}

// handleMemoryDetail serves GET /ui/memories/{id}.
func (h *Handler) handleMemoryDetail(w http.ResponseWriter, r *http.Request) {
	idStr := strings.TrimPrefix(r.URL.Path, "/ui/memories/")
	id, err := uuid.Parse(idStr)
	if err != nil {
		http.NotFound(w, r)
		return
	}

	if h.pool == nil {
		http.NotFound(w, r)
		return
	}

	mem, err := compat.GetMemory(r.Context(), h.pool, id)
	if err != nil || mem == nil {
		http.NotFound(w, r)
		return
	}

	h.render(w, r, "memory_detail", "Memory", struct{ Memory *db.Memory }{mem})
}

// handleMemoryForget serves POST /ui/memories/{id}/forget.
func (h *Handler) handleMemoryForget(w http.ResponseWriter, r *http.Request) {
	trimmed := strings.TrimPrefix(r.URL.Path, "/ui/memories/")
	idStr := strings.TrimSuffix(trimmed, "/forget")
	id, err := uuid.Parse(idStr)
	if err != nil {
		http.Error(w, "invalid memory id", http.StatusBadRequest)
		return
	}
	if h.pool == nil {
		http.Error(w, "service unavailable", http.StatusServiceUnavailable)
		return
	}
	if err := compat.SoftDeleteMemory(r.Context(), h.pool, id); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	http.Redirect(w, r, "/ui/memories", http.StatusSeeOther)
}
