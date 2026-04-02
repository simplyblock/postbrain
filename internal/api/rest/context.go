package rest

import (
	"net/http"
	"strconv"

	"github.com/google/uuid"

	"github.com/simplyblock/postbrain/internal/auth"
	"github.com/simplyblock/postbrain/internal/db"
	"github.com/simplyblock/postbrain/internal/knowledge"
	"github.com/simplyblock/postbrain/internal/memory"
)

func (ro *Router) getContext(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	scopeStr := q.Get("scope")
	query := q.Get("q")
	maxTokens := 4000
	if v := q.Get("max_tokens"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			maxTokens = n
		}
	}

	if scopeStr == "" {
		writeError(w, http.StatusBadRequest, "scope is required")
		return
	}

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
	if err := ro.authorizeRequestedScope(r.Context(), scope.ID); err != nil {
		writeScopeAuthzError(w, r, scope.ID, err)
		return
	}

	principalID, _ := r.Context().Value(auth.ContextKeyPrincipalID).(uuid.UUID)
	authorizedScopeIDs, err := ro.effectiveScopeIDsForRequest(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, "scope authorization failed")
		return
	}

	type contextBlock struct {
		Layer   string `json:"layer"`
		Type    string `json:"type,omitempty"`
		Title   string `json:"title,omitempty"`
		Content string `json:"content"`
	}

	var blocks []contextBlock
	totalTokens := 0
	estimateTokens := func(text string) int { return len(text) / 4 }

	if ro.knwStore != nil {
		arts, err := ro.knwStore.Recall(r.Context(), ro.pool, knowledge.RecallInput{
			Query:   query,
			ScopeID: scope.ID,
			Limit:   50,
		})
		if err == nil {
			for _, a := range arts {
				tokens := estimateTokens(a.Artifact.Content)
				if totalTokens+tokens > maxTokens {
					continue
				}
				blocks = append(blocks, contextBlock{
					Layer:   "knowledge",
					Type:    a.Artifact.KnowledgeType,
					Title:   a.Artifact.Title,
					Content: a.Artifact.Content,
				})
				totalTokens += tokens
			}
		}
	}

	if ro.memStore != nil {
		mems, err := ro.memStore.Recall(r.Context(), memory.RecallInput{
			Query:              query,
			ScopeID:            scope.ID,
			PrincipalID:        principalID,
			AuthorizedScopeIDs: authorizedScopeIDs,
			SearchMode:         "hybrid",
			Limit:              50,
		})
		if err == nil {
			for _, m := range mems {
				tokens := estimateTokens(m.Memory.Content)
				if totalTokens+tokens > maxTokens {
					continue
				}
				blocks = append(blocks, contextBlock{
					Layer:   "memory",
					Type:    m.Memory.MemoryType,
					Content: m.Memory.Content,
				})
				totalTokens += tokens
			}
		}
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"context_blocks": blocks,
		"total_tokens":   totalTokens,
	})
}
