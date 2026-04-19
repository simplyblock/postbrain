package ui

import (
	"html/template"
	"net/http"
	"strconv"

	"github.com/google/uuid"

	"github.com/simplyblock/postbrain/internal/retrieval"
)

// handleQuery serves GET /ui/{scope}/query — the recall playground.
func (h *Handler) handleQuery(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	_, scopeSet := h.authorizedScopesForRequest(ctx, r)
	scope := scopeFromContext(ctx)

	type queryResult struct {
		Layer         string
		ID            string
		Score         float64
		Title         string
		Content       string
		MemoryType    string
		KnowledgeType string
		ArtifactKind  string
		SourceRef     string
		Status        string
		Visibility    string
		Endorsements  int
	}

	data := struct {
		Title      string
		Content    template.HTML
		Query      string
		ScopeID    string
		Layers     map[string]bool
		SearchMode string
		Limit      int
		Results    []queryResult
		Ran        bool
		Error      string
	}{
		Title:      "Query Playground",
		Layers:     map[string]bool{"memory": true, "knowledge": true, "skill": true},
		SearchMode: "hybrid",
		Limit:      10,
	}
	if scope != nil {
		data.ScopeID = scope.ID.String()
	}

	if r.Method == http.MethodGet && r.URL.Query().Get("q") != "" {
		data.Query = r.URL.Query().Get("q")
		data.SearchMode = r.URL.Query().Get("search_mode")
		if data.SearchMode == "" {
			data.SearchMode = "hybrid"
		}
		if l, err := strconv.Atoi(r.URL.Query().Get("limit")); err == nil && l > 0 {
			data.Limit = l
		}
		// Layers from checkboxes.
		data.Layers = map[string]bool{
			"memory":    r.URL.Query().Get("layer_memory") == "1",
			"knowledge": r.URL.Query().Get("layer_knowledge") == "1",
			"skill":     r.URL.Query().Get("layer_skill") == "1",
		}
		// Default: all layers if none checked.
		if !data.Layers["memory"] && !data.Layers["knowledge"] && !data.Layers["skill"] {
			data.Layers = map[string]bool{"memory": true, "knowledge": true, "skill": true}
		}

		// Scope is pre-validated by dispatchScopedRoute; use it directly.
		var scopeID uuid.UUID
		if scope != nil {
			scopeID = scope.ID
		}

		principalID := h.principalFromCookie(r)
		authorizedScopeIDs := make([]uuid.UUID, 0, len(scopeSet))
		for id := range scopeSet {
			authorizedScopeIDs = append(authorizedScopeIDs, id)
		}

		activeLayers := map[retrieval.Layer]bool{
			retrieval.LayerMemory:    data.Layers["memory"],
			retrieval.LayerKnowledge: data.Layers["knowledge"],
			retrieval.LayerSkill:     data.Layers["skill"],
		}

		merged, err := retrieval.OrchestrateRecall(ctx, retrieval.OrchestrateDeps{
			Pool:     h.pool,
			MemStore: h.memStore,
			KnwStore: h.knwStore,
			Svc:      h.svc,
		}, retrieval.OrchestrateInput{
			Query:              data.Query,
			ScopeID:            scopeID,
			PrincipalID:        principalID,
			AuthorizedScopeIDs: authorizedScopeIDs,
			SearchMode:         data.SearchMode,
			Limit:              data.Limit,
			MinScore:           0,
			GraphDepth:         1,
			ActiveLayers:       activeLayers,
		})
		if err != nil {
			data.Error = "query recall: " + err.Error()
		}
		for _, res := range merged {
			content := res.Content
			if res.Layer == retrieval.LayerSkill && content == "" {
				content = res.Description
			}
			title := res.Title
			if res.Layer == retrieval.LayerSkill {
				title = res.Name
			}
			data.Results = append(data.Results, queryResult{
				Layer:         string(res.Layer),
				ID:            res.ID.String(),
				Score:         res.Score,
				Title:         title,
				Content:       content,
				MemoryType:    res.MemoryType,
				KnowledgeType: res.KnowledgeType,
				ArtifactKind:  res.ArtifactKind,
				SourceRef:     res.SourceRef,
				Status:        res.Status,
				Visibility:    res.Visibility,
				Endorsements:  res.Endorsements,
			})
		}
		data.Ran = true
	}

	h.render(w, r, "query", "Query Playground", data)
}

// handleMetrics serves GET /ui/metrics.
func (h *Handler) handleMetrics(w http.ResponseWriter, r *http.Request) {
	h.render(w, r, "metrics", "Metrics", nil)
}
