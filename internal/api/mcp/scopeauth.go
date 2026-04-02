package mcp

import (
	"context"
	"errors"

	"github.com/google/uuid"
	mcpgo "github.com/mark3labs/mcp-go/mcp"

	"github.com/simplyblock/postbrain/internal/api/scopeauth"
)

func (s *Server) authorizeRequestedScope(ctx context.Context, requestedScopeID uuid.UUID) error {
	return scopeauth.AuthorizeContextScope(ctx, s.membership, requestedScopeID)
}

func scopeAuthzToolError(err error) *mcpgo.CallToolResult {
	switch {
	case errors.Is(err, scopeauth.ErrTokenScopeDenied),
		errors.Is(err, scopeauth.ErrPrincipalScopeDenied),
		errors.Is(err, scopeauth.ErrMissingToken),
		errors.Is(err, scopeauth.ErrMissingPrincipal):
		return mcpgo.NewToolResultError("forbidden: scope access denied")
	default:
		return mcpgo.NewToolResultError("scope authorization failed")
	}
}
