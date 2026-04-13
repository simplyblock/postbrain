package ui

import (
	"encoding/json"
	"html/template"
	"net/http"

	"github.com/google/uuid"

	"github.com/simplyblock/postbrain/internal/db"
	"github.com/simplyblock/postbrain/internal/db/compat"
)

// graphNode is the JSON shape consumed by the D3 force simulation.
type graphNode struct {
	ID   string `json:"id"`
	Name string `json:"name"`
	Type string `json:"type"`
}

// graphLink is the JSON shape for a relation edge.
type graphLink struct {
	Source     string  `json:"source"`
	Target     string  `json:"target"`
	Predicate  string  `json:"predicate"`
	Confidence float64 `json:"confidence"`
}

// handleGraph serves GET /ui/graph.
func (h *Handler) handleGraph(w http.ResponseWriter, r *http.Request) {
	data := h.graphViewData(r, r.URL.Query().Get("scope_id"))
	h.render(w, r, "graph", "Entity Graph", data)
}

// handleGraph3D serves GET /ui/graph3d.
func (h *Handler) handleGraph3D(w http.ResponseWriter, r *http.Request) {
	data := h.graphViewData(r, r.URL.Query().Get("scope_id"))
	h.render(w, r, "graph3d", "Entity Graph 3D", data)
}

func (h *Handler) graphViewData(r *http.Request, scopeStr string) struct {
	Scopes    []*db.Scope
	ScopeID   string
	NodeCount int
	EdgeCount int
	GraphJSON template.JS
} {
	data := struct {
		Scopes    []*db.Scope
		ScopeID   string
		NodeCount int
		EdgeCount int
		GraphJSON template.JS
	}{ScopeID: scopeStr}

	if h.pool == nil {
		return data
	}

	scopes, scopeSet := h.authorizedScopesForRequest(r.Context(), r)
	data.Scopes = scopes

	// Default to first scope when none is selected.
	if scopeStr == "" && len(data.Scopes) > 0 {
		scopeStr = data.Scopes[0].ID.String()
		data.ScopeID = scopeStr
	}
	if scopeStr != "" {
		if sid, err := uuid.Parse(scopeStr); err == nil {
			if _, ok := scopeSet[sid]; !ok {
				if len(data.Scopes) > 0 {
					scopeStr = data.Scopes[0].ID.String()
					data.ScopeID = scopeStr
				} else {
					scopeStr = ""
					data.ScopeID = ""
				}
			}
		}
	}

	if scopeStr != "" {
		sid, err := uuid.Parse(scopeStr)
		if err == nil {
			nodes := []graphNode{}
			links := []graphLink{}

			ents, err := compat.ListEntitiesByScope(r.Context(), h.pool, sid, "", 100000, 0)
			if err == nil {
				for _, e := range ents {
					nodes = append(nodes, graphNode{
						ID:   e.ID.String(),
						Name: e.Name,
						Type: e.EntityType,
					})
				}
			}

			nodeIDs := make(map[string]bool, len(nodes))
			for _, n := range nodes {
				nodeIDs[n.ID] = true
			}

			if rels, err := compat.ListRelationsByScope(r.Context(), h.pool, sid); err == nil {
				for _, rel := range rels {
					src, tgt := rel.SubjectID.String(), rel.ObjectID.String()
					if !nodeIDs[src] || !nodeIDs[tgt] {
						continue // skip dangling relations
					}
					links = append(links, graphLink{
						Source:     src,
						Target:     tgt,
						Predicate:  rel.Predicate,
						Confidence: rel.Confidence,
					})
				}
			}

			data.NodeCount = len(nodes)
			data.EdgeCount = len(links)

			payload, err := json.Marshal(map[string]any{"nodes": nodes, "links": links})
			if err == nil {
				data.GraphJSON = template.JS(payload)
			}
		}
	}

	return data
}
