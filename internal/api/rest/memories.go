package rest

import (
	"context"
	"net/http"

	"github.com/google/uuid"

	"github.com/simplyblock/postbrain/internal/auth"
	"github.com/simplyblock/postbrain/internal/db"
	"github.com/simplyblock/postbrain/internal/knowledge"
	"github.com/simplyblock/postbrain/internal/memory"
)

type entityRequest struct {
	Name string `json:"name"`
	Type string `json:"type"`
}

type createMemoryRequest struct {
	Content    string          `json:"content"`
	Summary    *string         `json:"summary"`
	MemoryType string          `json:"memory_type"`
	Scope      string          `json:"scope"`
	Importance float64         `json:"importance"`
	SourceRef  *string         `json:"source_ref"`
	Entities   []entityRequest `json:"entities"`
	ExpiresIn  *int            `json:"expires_in"`
}

func entityRequestsToInput(reqs []entityRequest) []memory.EntityInput {
	out := make([]memory.EntityInput, 0, len(reqs))
	for _, e := range reqs {
		if e.Name != "" {
			out = append(out, memory.EntityInput{Name: e.Name, Type: e.Type})
		}
	}
	return out
}

func (ro *Router) createMemory(w http.ResponseWriter, r *http.Request) {
	var body createMemoryRequest
	if err := readJSON(r, &body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if body.Content == "" {
		writeError(w, http.StatusBadRequest, "content is required")
		return
	}
	if body.Scope == "" {
		writeError(w, http.StatusBadRequest, "scope is required")
		return
	}

	kind, externalID, err := parseScopeString(body.Scope)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	scope, err := db.GetScopeByExternalID(r.Context(), ro.pool, kind, externalID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "scope lookup failed")
		return
	}
	if scope == nil {
		writeError(w, http.StatusBadRequest, "scope not found")
		return
	}
	if err := ro.authorizeRequestedScope(r.Context(), scope.ID); err != nil {
		writeScopeAuthzError(w, r, scope.ID, err)
		return
	}

	principalID, _ := r.Context().Value(auth.ContextKeyPrincipalID).(uuid.UUID)

	memoryType := body.MemoryType
	if memoryType == "" {
		memoryType = "semantic"
	}
	importance := body.Importance
	if importance == 0 {
		importance = 0.5
	}

	result, err := ro.memStore.Create(r.Context(), memory.CreateInput{
		Content:    body.Content,
		Summary:    body.Summary,
		MemoryType: memoryType,
		ScopeID:    scope.ID,
		AuthorID:   principalID,
		Importance: importance,
		SourceRef:  body.SourceRef,
		Entities:   entityRequestsToInput(body.Entities),
		ExpiresIn:  body.ExpiresIn,
	})
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusCreated, map[string]any{
		"memory_id": result.MemoryID,
		"action":    result.Action,
	})
}

func (ro *Router) recallMemories(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	query := q.Get("q")
	if query == "" {
		writeError(w, http.StatusBadRequest, "q is required")
		return
	}
	scopeStr := q.Get("scope")
	pg := paginationFromRequest(r)

	principalID, _ := r.Context().Value(auth.ContextKeyPrincipalID).(uuid.UUID)
	authorizedScopeIDs, err := ro.effectiveScopeIDsForRequest(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, "scope authorization failed")
		return
	}

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
		if err := ro.authorizeRequestedScope(r.Context(), scope.ID); err != nil {
			writeScopeAuthzError(w, r, scope.ID, err)
			return
		}
		scopeID = scope.ID
	}

	results, err := ro.memStore.Recall(r.Context(), memory.RecallInput{
		Query:              query,
		ScopeID:            scopeID,
		PrincipalID:        principalID,
		AuthorizedScopeIDs: authorizedScopeIDs,
		Limit:              pg.Limit,
	})
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"results": results})
}

func (ro *Router) getMemory(w http.ResponseWriter, r *http.Request) {
	id, err := uuidParam(r, "id")
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid memory id")
		return
	}
	m, err := db.GetMemory(r.Context(), ro.pool, id)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if m == nil {
		writeError(w, http.StatusNotFound, "memory not found")
		return
	}
	if err := ro.authorizeObjectScope(r.Context(), m.ScopeID); err != nil {
		writeScopeAuthzError(w, r, m.ScopeID, err)
		return
	}
	writeJSON(w, http.StatusOK, m)
}

type updateMemoryRequest struct {
	Content    string  `json:"content"`
	Summary    *string `json:"summary"`
	Importance float64 `json:"importance"`
}

func (ro *Router) updateMemory(w http.ResponseWriter, r *http.Request) {
	id, err := uuidParam(r, "id")
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid memory id")
		return
	}
	var body updateMemoryRequest
	if err := readJSON(r, &body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	existing, err := db.GetMemory(r.Context(), ro.pool, id)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if existing == nil {
		writeError(w, http.StatusNotFound, "memory not found")
		return
	}
	if err := ro.authorizeObjectScope(r.Context(), existing.ScopeID); err != nil {
		writeScopeAuthzError(w, r, existing.ScopeID, err)
		return
	}
	updated, err := ro.memStore.Update(r.Context(), id, body.Content, body.Summary, body.Importance)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, updated)
}

func (ro *Router) deleteMemory(w http.ResponseWriter, r *http.Request) {
	id, err := uuidParam(r, "id")
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid memory id")
		return
	}
	hard := r.URL.Query().Get("hard") == "true"
	existing, err := db.GetMemory(r.Context(), ro.pool, id)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if existing == nil {
		writeError(w, http.StatusNotFound, "memory not found")
		return
	}
	if err := ro.authorizeDeleteObjectScope(r.Context(), existing.ScopeID); err != nil {
		writeScopeAuthzError(w, r, existing.ScopeID, err)
		return
	}
	if hard {
		if err := ro.memStore.HardDelete(r.Context(), id); err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"memory_id": id, "action": "deleted"})
		return
	}
	if err := ro.memStore.SoftDelete(r.Context(), id); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"memory_id": id, "action": "deactivated"})
}

type promoteMemoryRequest struct {
	TargetScope      string  `json:"target_scope"`
	TargetVisibility string  `json:"target_visibility"`
	ProposedTitle    *string `json:"proposed_title"`
	CollectionSlug   string  `json:"collection_slug"`
}

func (ro *Router) promoteMemory(w http.ResponseWriter, r *http.Request) {
	memID, err := uuidParam(r, "id")
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid memory id")
		return
	}
	var body promoteMemoryRequest
	if err := readJSON(r, &body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if body.TargetScope == "" || body.TargetVisibility == "" {
		writeError(w, http.StatusBadRequest, "target_scope and target_visibility are required")
		return
	}

	kind, externalID, err := parseScopeString(body.TargetScope)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	scope, err := db.GetScopeByExternalID(r.Context(), ro.pool, kind, externalID)
	if err != nil || scope == nil {
		writeError(w, http.StatusBadRequest, "target scope not found")
		return
	}
	if err := ro.authorizeRequestedScope(r.Context(), scope.ID); err != nil {
		writeScopeAuthzError(w, r, scope.ID, err)
		return
	}

	principalID, _ := r.Context().Value(auth.ContextKeyPrincipalID).(uuid.UUID)

	var collID *uuid.UUID
	if body.CollectionSlug != "" && ro.knwColl != nil {
		coll, err := ro.knwColl.GetBySlug(r.Context(), scope.ID, body.CollectionSlug)
		if err == nil && coll != nil {
			collID = &coll.ID
		}
	}

	req, err := ro.knwProm.CreateRequest(r.Context(), knowledge.PromoteInput{
		MemoryID:             memID,
		RequestedBy:          principalID,
		TargetScopeID:        scope.ID,
		TargetVisibility:     body.TargetVisibility,
		ProposedTitle:        body.ProposedTitle,
		ProposedCollectionID: collID,
	})
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusCreated, req)
}

type summarizeMemoriesRequest struct {
	Scope  string `json:"scope"`
	Topic  string `json:"topic"`
	DryRun bool   `json:"dry_run"`
}

// POST /v1/memories/summarize consolidates episodic memories in the given scope.
func (ro *Router) handleSummarizeMemories(w http.ResponseWriter, req *http.Request) {
	var input summarizeMemoriesRequest
	if err := readJSON(req, &input); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	if input.Scope == "" {
		writeError(w, http.StatusBadRequest, "scope is required")
		return
	}

	kind, externalID, err := parseScopeString(input.Scope)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	scope, err := db.GetScopeByExternalID(req.Context(), ro.pool, kind, externalID)
	if err != nil || scope == nil {
		writeError(w, http.StatusBadRequest, "scope not found")
		return
	}
	if err := ro.authorizeRequestedScope(req.Context(), scope.ID); err != nil {
		writeScopeAuthzError(w, req, scope.ID, err)
		return
	}

	if ro.consolidator == nil {
		writeError(w, http.StatusServiceUnavailable, "consolidation not available")
		return
	}

	clusters, err := ro.consolidator.FindClusters(req.Context(), scope.ID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	if input.DryRun || len(clusters) == 0 {
		writeJSON(w, http.StatusOK, map[string]any{
			"consolidated_count": 0,
			"cluster_count":      len(clusters),
			"dry_run":            input.DryRun,
		})
		return
	}

	// Merge all clusters using a simple concatenation summarizer.
	var consolidated int
	var lastMemID *uuid.UUID
	for _, cluster := range clusters {
		merged, err := ro.consolidator.MergeCluster(req.Context(), cluster, func(ctx context.Context, contents []string) (string, error) {
			// Simple summarizer: join contents. Replace with LLM summarizer when available.
			result := ""
			for i, c := range contents {
				if i > 0 {
					result += " | "
				}
				result += c
			}
			return result, nil
		})
		if err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
		consolidated += len(cluster)
		id := merged.ID
		lastMemID = &id
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"consolidated_count": consolidated,
		"result_memory_id":   lastMemID,
		"cluster_count":      len(clusters),
	})
}

// parseScopeString is duplicated here to avoid a cross-package dependency.
// It splits "kind:external_id" into parts.
func parseScopeString(scope string) (string, string, error) {
	if scope == "" {
		return "", "", errString("empty scope string")
	}
	for i, c := range scope {
		if c == ':' {
			return scope[:i], scope[i+1:], nil
		}
	}
	return "", "", errString("missing ':' separator in scope: " + scope)
}

type errString string

func (e errString) Error() string { return string(e) }
