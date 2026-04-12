package mcp

import (
	"context"
	"errors"
	"fmt"
	"log/slog"

	"github.com/google/uuid"
	mcpgo "github.com/mark3labs/mcp-go/mcp"

	"github.com/simplyblock/postbrain/internal/api/scopeauth"
	"github.com/simplyblock/postbrain/internal/auth"
	"github.com/simplyblock/postbrain/internal/authz"
	"github.com/simplyblock/postbrain/internal/db"
	"github.com/simplyblock/postbrain/internal/metrics"
)

// resolveScope parses a "kind:external_id" string, looks up the scope in the DB,
// and checks authorization. On failure it returns a non-nil *CallToolResult error
// and uuid.Nil; on success it returns the scope UUID and a nil result.
func (s *Server) resolveScope(ctx context.Context, toolName, scopeStr string) (uuid.UUID, *mcpgo.CallToolResult) {
	kind, externalID, err := parseScopeString(scopeStr)
	if err != nil {
		return uuid.Nil, mcpgo.NewToolResultError(fmt.Sprintf("%s: invalid scope: %v", toolName, err))
	}
	scope, err := db.GetScopeByExternalID(ctx, s.pool, kind, externalID)
	if err != nil {
		return uuid.Nil, mcpgo.NewToolResultError(fmt.Sprintf("%s: scope lookup: %v", toolName, err))
	}
	if scope == nil {
		return uuid.Nil, mcpgo.NewToolResultError(fmt.Sprintf("%s: scope '%s' not found", toolName, scopeStr))
	}
	if err := s.authorizeRequestedScope(ctx, scope.ID); err != nil {
		return uuid.Nil, scopeAuthzToolError(ctx, toolName, scope.ID, err)
	}
	return scope.ID, nil
}

func (s *Server) authorizeRequestedScope(ctx context.Context, requestedScopeID uuid.UUID) error {
	perm, _ := ctx.Value(contextKeyToolPermission{}).(authz.Permission)
	if perm == "" {
		perm = "scopes:read"
	}
	return scopeauth.AuthorizeContextScope(ctx, requestedScopeID, perm)
}

// authorizeDeleteObjectScope enforces delete semantics: a caller may only delete
// objects in scopes directly owned by the caller principal (never in ancestor scopes).
func (s *Server) authorizeDeleteObjectScope(ctx context.Context, objectScopeID uuid.UUID) error {
	if err := s.authorizeRequestedScope(ctx, objectScopeID); err != nil {
		return err
	}
	scope, err := db.GetScopeByID(ctx, s.pool, objectScopeID)
	if err != nil {
		return err
	}
	if scope == nil {
		return fmt.Errorf("%w: scope not found", scopeauth.ErrPrincipalScopeDenied)
	}
	principalID, _ := ctx.Value(auth.ContextKeyPrincipalID).(uuid.UUID)
	if principalID == uuid.Nil {
		return fmt.Errorf("%w", scopeauth.ErrMissingPrincipal)
	}
	if scope.PrincipalID != principalID {
		return fmt.Errorf("%w: delete not allowed in ancestor scope", scopeauth.ErrPrincipalScopeDenied)
	}
	return nil
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
	principalID, _ := ctx.Value(auth.ContextKeyPrincipalID).(uuid.UUID)
	if principalID == uuid.Nil {
		return nil, nil
	}

	// Use the DB resolver's ReachableScopeIDs when available.
	tokenResolver, _ := ctx.Value(auth.ContextKeyTokenResolver).(*authz.TokenResolver)
	if tokenResolver != nil {
		if dbr := tokenResolver.DBResolver(); dbr != nil {
			return dbr.ReachableScopeIDs(ctx, principalID)
		}
	}

	// Fallback: membership-only scope resolution.
	if s.membership == nil {
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
	if token == nil || len(token.ScopeIds) == 0 {
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
