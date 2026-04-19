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

// handleMemories serves GET /ui/{scope}/memories.
func (h *Handler) handleMemories(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query().Get("q")
	offset := 0
	if c := r.URL.Query().Get("cursor"); c != "" {
		if v, err := strconv.Atoi(c); err == nil && v > 0 {
			offset = v
		}
	}

	scope := scopeFromContext(r.Context())

	data := struct {
		Query      string
		ScopeID    string
		Memories   []*db.Memory
		NextCursor string
	}{
		Query: q,
	}
	if scope != nil {
		data.ScopeID = scope.ID.String()
	}

	if h.pool != nil && scope != nil {
		mems, err := compat.ListMemoriesByScope(r.Context(), h.pool, scope.ID, memoriesPageSize+1, offset)
		if err == nil {
			if len(mems) > memoriesPageSize {
				data.Memories = mems[:memoriesPageSize]
				data.NextCursor = strconv.Itoa(offset + memoriesPageSize)
			} else {
				data.Memories = mems
			}
		}
	}

	tmpl := "memories"
	if r.Header.Get("HX-Request") == "true" {
		tmpl = "memories_rows"
	}
	h.render(w, r, tmpl, "Memories", data)
}

// handleMemoryDetail serves GET /ui/{scope}/memories/{id}.
func (h *Handler) handleMemoryDetail(w http.ResponseWriter, r *http.Request) {
	path := routePathFromContext(r.Context(), r)
	idStr := strings.TrimPrefix(path, "/ui/memories/")
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
	scope := scopeFromContext(r.Context())
	if scope == nil || mem.ScopeID != scope.ID {
		http.NotFound(w, r)
		return
	}
	h.render(w, r, "memory_detail", "Memory", struct {
		Memory  *db.Memory
		ScopeID string
	}{mem, scope.ID.String()})
}

// handleMemoryForget serves POST /ui/{scope}/memories/{id}/forget.
func (h *Handler) handleMemoryForget(w http.ResponseWriter, r *http.Request) {
	path := routePathFromContext(r.Context(), r)
	trimmed := strings.TrimPrefix(path, "/ui/memories/")
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
	// Verify the memory belongs to the current scope before deleting.
	// Return 404 regardless of whether the memory exists or is in a different
	// scope to avoid leaking cross-scope existence.
	scope := scopeFromContext(r.Context())
	mem, err := compat.GetMemory(r.Context(), h.pool, id)
	if err != nil || mem == nil || scope == nil || mem.ScopeID != scope.ID {
		http.NotFound(w, r)
		return
	}
	if err := compat.SoftDeleteMemory(r.Context(), h.pool, id); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	scopedRedirect(w, r, "/memories")
}
