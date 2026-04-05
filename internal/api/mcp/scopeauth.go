package mcp

import (
	"context"
	"errors"
	"log/slog"

	"github.com/google/uuid"
	mcpgo "github.com/mark3labs/mcp-go/mcp"

	"github.com/simplyblock/postbrain/internal/api/scopeauth"
	"github.com/simplyblock/postbrain/internal/auth"
	"github.com/simplyblock/postbrain/internal/db"
	"github.com/simplyblock/postbrain/internal/metrics"
)

func (s *Server) authorizeRequestedScope(ctx context.Context, requestedScopeID uuid.UUID) error {
	return scopeauth.AuthorizeContextScope(ctx, s.membership, requestedScopeID)
}

func scopeAuthzToolError(ctx context.Context, tool string, requestedScopeID uuid.UUID, err error) *mcpgo.CallToolResult {
	switch {
	case errors.Is(err, scopeauth.ErrTokenScopeDenied),
		errors.Is(err, scopeauth.ErrPrincipalScopeDenied),
		errors.Is(err, scopeauth.ErrMissingToken),
		errors.Is(err, scopeauth.ErrMissingPrincipal):
		logMCPScopeAuthzDenied(ctx, tool, requestedScopeID)
		return mcpgo.NewToolResultError("forbidden: scope access denied")
	default:
		return mcpgo.NewToolResultError("scope authorization failed")
	}
}

func logMCPScopeAuthzDenied(ctx context.Context, tool string, requestedScopeID uuid.UUID) {
	fields := []any{
		"surface", "mcp",
		"endpoint", tool,
		"requested_scope_id", requestedScopeID,
	}
	if principalID, _ := ctx.Value(auth.ContextKeyPrincipalID).(uuid.UUID); principalID != uuid.Nil {
		fields = append(fields, "principal_id", principalID)
	}
	if token, _ := ctx.Value(auth.ContextKeyToken).(*db.Token); token != nil {
		fields = append(fields, "token_id", token.ID)
	}
	slog.WarnContext(ctx, "scope access denied", fields...)
	metrics.ScopeAuthzDenied.WithLabelValues("mcp", tool).Inc()
}

func (s *Server) effectiveScopeIDsForRequest(ctx context.Context) ([]uuid.UUID, error) {
	if ids, ok := scopeauth.EffectiveScopeIDsFromContext(ctx); ok {
		return ids, nil
	}
	if s.membership == nil {
		return nil, nil
	}
	principalID, _ := ctx.Value(auth.ContextKeyPrincipalID).(uuid.UUID)
	if principalID == uuid.Nil {
		return nil, nil
	}
	return s.membership.EffectiveScopeIDs(ctx, principalID)
}

func (s *Server) authorizedScopeIDsForRequest(ctx context.Context) ([]uuid.UUID, error) {
	effectiveScopeIDs, err := s.effectiveScopeIDsForRequest(ctx)
	if err != nil {
		return nil, err
	}
	token, _ := ctx.Value(auth.ContextKeyToken).(*db.Token)
	if token == nil || token.ScopeIds == nil {
		return effectiveScopeIDs, nil
	}
	allowedByToken := make(map[uuid.UUID]struct{}, len(token.ScopeIds))
	for _, id := range token.ScopeIds {
		allowedByToken[id] = struct{}{}
	}
	authorized := make([]uuid.UUID, 0, len(effectiveScopeIDs))
	for _, id := range effectiveScopeIDs {
		if _, ok := allowedByToken[id]; ok {
			authorized = append(authorized, id)
		}
	}
	return authorized, nil
}
