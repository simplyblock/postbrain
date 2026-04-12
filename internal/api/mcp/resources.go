package mcp

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	mcpgo "github.com/mark3labs/mcp-go/mcp"

	"github.com/simplyblock/postbrain/internal/db"
)

// registerResources registers all MCP resource templates.
func (s *Server) registerResources() {
	s.mcpServer.AddResourceTemplate(
		mcpgo.NewResourceTemplate(
			"postbrain://memory/{id}",
			"Postbrain memory item",
			mcpgo.WithTemplateDescription("A single memory item identified by UUID. Includes content, summary, confidence, and provenance fields. Embeddings are omitted."),
			mcpgo.WithTemplateMIMEType("application/json"),
		),
		s.handleMemoryResource,
	)
	s.mcpServer.AddResourceTemplate(
		mcpgo.NewResourceTemplate(
			"postbrain://knowledge/{id}",
			"Postbrain knowledge artifact",
			mcpgo.WithTemplateDescription("A published knowledge artifact identified by UUID. Includes title, content, summary, endorsement count, and publication metadata. Embeddings are omitted."),
			mcpgo.WithTemplateMIMEType("application/json"),
		),
		s.handleKnowledgeResource,
	)
	s.mcpServer.AddResourceTemplate(
		mcpgo.NewResourceTemplate(
			"postbrain://session/{id}",
			"Postbrain session",
			mcpgo.WithTemplateDescription("A postbrain session identified by UUID. Includes scope, principal, start/end times, and attached metadata."),
			mcpgo.WithTemplateMIMEType("application/json"),
		),
		s.handleSessionResource,
	)
}

// handleMemoryResource serves postbrain://memory/<uuid>.
func (s *Server) handleMemoryResource(ctx context.Context, req mcpgo.ReadResourceRequest) ([]mcpgo.ResourceContents, error) {
	id, err := parseResourceUUID(req.Params.URI, "postbrain://memory/")
	if err != nil {
		return nil, err
	}
	if s.pool == nil {
		return nil, fmt.Errorf("memory resource: server not configured")
	}
	mem, err := db.GetMemory(ctx, s.pool, id)
	if err != nil {
		return nil, fmt.Errorf("memory resource: fetch: %w", err)
	}
	if mem == nil {
		return nil, fmt.Errorf("memory resource: %s not found", id)
	}
	if err := s.authorizeRequestedScope(ctx, mem.ScopeID); err != nil {
		return nil, fmt.Errorf("memory resource: %w", err)
	}

	type memView struct {
		ID              uuid.UUID `json:"id"`
		MemoryType      string    `json:"memory_type"`
		ScopeID         uuid.UUID `json:"scope_id"`
		ContentKind     string    `json:"content_kind"`
		Content         string    `json:"content"`
		Summary         *string   `json:"summary,omitempty"`
		Confidence      float64   `json:"confidence"`
		Importance      float64   `json:"importance"`
		PromotionStatus string    `json:"promotion_status"`
		IsActive        bool      `json:"is_active"`
		SourceRef       *string   `json:"source_ref,omitempty"`
		CreatedAt       time.Time `json:"created_at"`
		UpdatedAt       time.Time `json:"updated_at"`
	}
	out, err := json.Marshal(memView{
		ID:              mem.ID,
		MemoryType:      mem.MemoryType,
		ScopeID:         mem.ScopeID,
		ContentKind:     mem.ContentKind,
		Content:         mem.Content,
		Summary:         mem.Summary,
		Confidence:      mem.Confidence,
		Importance:      mem.Importance,
		PromotionStatus: mem.PromotionStatus,
		IsActive:        mem.IsActive,
		SourceRef:       mem.SourceRef,
		CreatedAt:       mem.CreatedAt,
		UpdatedAt:       mem.UpdatedAt,
	})
	if err != nil {
		return nil, fmt.Errorf("memory resource: marshal: %w", err)
	}
	return []mcpgo.ResourceContents{
		mcpgo.TextResourceContents{URI: req.Params.URI, MIMEType: "application/json", Text: string(out)},
	}, nil
}

// handleKnowledgeResource serves postbrain://knowledge/<uuid>.
func (s *Server) handleKnowledgeResource(ctx context.Context, req mcpgo.ReadResourceRequest) ([]mcpgo.ResourceContents, error) {
	id, err := parseResourceUUID(req.Params.URI, "postbrain://knowledge/")
	if err != nil {
		return nil, err
	}
	if s.pool == nil {
		return nil, fmt.Errorf("knowledge resource: server not configured")
	}
	artifact, err := db.GetArtifact(ctx, s.pool, id)
	if err != nil {
		return nil, fmt.Errorf("knowledge resource: fetch: %w", err)
	}
	if artifact == nil {
		return nil, fmt.Errorf("knowledge resource: %s not found", id)
	}
	if err := s.authorizeRequestedScope(ctx, artifact.OwnerScopeID); err != nil {
		return nil, fmt.Errorf("knowledge resource: %w", err)
	}

	type artifactView struct {
		ID               uuid.UUID  `json:"id"`
		KnowledgeType    string     `json:"knowledge_type"`
		ArtifactKind     string     `json:"artifact_kind"`
		OwnerScopeID     uuid.UUID  `json:"owner_scope_id"`
		Visibility       string     `json:"visibility"`
		Status           string     `json:"status"`
		Title            string     `json:"title"`
		Content          string     `json:"content"`
		Summary          *string    `json:"summary,omitempty"`
		EndorsementCount int32      `json:"endorsement_count"`
		Version          int32      `json:"version"`
		SourceRef        *string    `json:"source_ref,omitempty"`
		PublishedAt      *time.Time `json:"published_at,omitempty"`
		CreatedAt        time.Time  `json:"created_at"`
		UpdatedAt        time.Time  `json:"updated_at"`
	}
	out, err := json.Marshal(artifactView{
		ID:               artifact.ID,
		KnowledgeType:    artifact.KnowledgeType,
		ArtifactKind:     artifact.ArtifactKind,
		OwnerScopeID:     artifact.OwnerScopeID,
		Visibility:       artifact.Visibility,
		Status:           artifact.Status,
		Title:            artifact.Title,
		Content:          artifact.Content,
		Summary:          artifact.Summary,
		EndorsementCount: artifact.EndorsementCount,
		Version:          artifact.Version,
		SourceRef:        artifact.SourceRef,
		PublishedAt:      artifact.PublishedAt,
		CreatedAt:        artifact.CreatedAt,
		UpdatedAt:        artifact.UpdatedAt,
	})
	if err != nil {
		return nil, fmt.Errorf("knowledge resource: marshal: %w", err)
	}
	return []mcpgo.ResourceContents{
		mcpgo.TextResourceContents{URI: req.Params.URI, MIMEType: "application/json", Text: string(out)},
	}, nil
}

// handleSessionResource serves postbrain://session/<uuid>.
func (s *Server) handleSessionResource(ctx context.Context, req mcpgo.ReadResourceRequest) ([]mcpgo.ResourceContents, error) {
	id, err := parseResourceUUID(req.Params.URI, "postbrain://session/")
	if err != nil {
		return nil, err
	}
	if s.pool == nil {
		return nil, fmt.Errorf("session resource: server not configured")
	}
	sess, err := db.GetSession(ctx, s.pool, id)
	if err != nil {
		return nil, fmt.Errorf("session resource: fetch: %w", err)
	}
	if sess == nil {
		return nil, fmt.Errorf("session resource: %s not found", id)
	}
	if err := s.authorizeRequestedScope(ctx, sess.ScopeID); err != nil {
		return nil, fmt.Errorf("session resource: %w", err)
	}

	type sessionView struct {
		ID          uuid.UUID  `json:"id"`
		ScopeID     uuid.UUID  `json:"scope_id"`
		PrincipalID *uuid.UUID `json:"principal_id,omitempty"`
		StartedAt   time.Time  `json:"started_at"`
		EndedAt     *time.Time `json:"ended_at,omitempty"`
	}
	out, err := json.Marshal(sessionView{
		ID:          sess.ID,
		ScopeID:     sess.ScopeID,
		PrincipalID: sess.PrincipalID,
		StartedAt:   sess.StartedAt,
		EndedAt:     sess.EndedAt,
	})
	if err != nil {
		return nil, fmt.Errorf("session resource: marshal: %w", err)
	}
	return []mcpgo.ResourceContents{
		mcpgo.TextResourceContents{URI: req.Params.URI, MIMEType: "application/json", Text: string(out)},
	}, nil
}

// parseResourceUUID strips the given prefix from uri and parses the remaining
// segment as a UUID. Returns a descriptive error if the segment is missing or
// malformed.
func parseResourceUUID(uri, prefix string) (uuid.UUID, error) {
	raw := strings.TrimPrefix(uri, prefix)
	if raw == "" || raw == uri {
		return uuid.Nil, fmt.Errorf("resource URI %q: missing ID segment", uri)
	}
	id, err := uuid.Parse(raw)
	if err != nil {
		return uuid.Nil, fmt.Errorf("resource URI %q: invalid UUID: %w", uri, err)
	}
	return id, nil
}
