package principals

import (
	"context"
	"errors"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/simplyblock/postbrain/internal/db"
)

// Sentinel errors returned by MembershipStore.
var (
	ErrCycleDetected = errors.New("principals: membership would create a cycle")
	ErrInvalidRole   = errors.New("principals: invalid role; must be member, owner, or admin")
)

var validRoles = map[string]bool{
	"member": true,
	"owner":  true,
	"admin":  true,
}

// MembershipStore manages principal memberships.
type MembershipStore struct {
	pool *pgxpool.Pool
}

// NewMembershipStore creates a new MembershipStore backed by the given pool.
func NewMembershipStore(pool *pgxpool.Pool) *MembershipStore {
	return &MembershipStore{pool: pool}
}

// AddMembership adds a membership from memberID to parentID after cycle detection.
// Returns ErrInvalidRole for unrecognized roles.
// Returns ErrCycleDetected if the addition would create a membership cycle.
func (m *MembershipStore) AddMembership(ctx context.Context, memberID, parentID uuid.UUID, role string, grantedBy *uuid.UUID) error {
	if !validRoles[role] {
		return ErrInvalidRole
	}

	// Cycle detection: if memberID already appears in the ancestor chain of parentID,
	// inserting memberID→parentID would form a cycle.
	ancestors, err := db.GetAllParentIDs(ctx, m.pool, parentID)
	if err != nil {
		return fmt.Errorf("principals: cycle check: %w", err)
	}
	for _, id := range ancestors {
		if id == memberID {
			return ErrCycleDetected
		}
	}

	_, err = db.CreateMembership(ctx, m.pool, memberID, parentID, role, grantedBy)
	if err != nil {
		return fmt.Errorf("principals: add membership: %w", err)
	}
	return nil
}

// RemoveMembership removes a direct membership between memberID and parentID.
func (m *MembershipStore) RemoveMembership(ctx context.Context, memberID, parentID uuid.UUID) error {
	if err := db.DeleteMembership(ctx, m.pool, memberID, parentID); err != nil {
		return fmt.Errorf("principals: remove membership: %w", err)
	}
	return nil
}

// EffectiveScopeIDs returns all scope IDs accessible to a principal via memberships.
// This includes scopes owned by the principal itself and by all ancestor principals.
func (m *MembershipStore) EffectiveScopeIDs(ctx context.Context, principalID uuid.UUID) ([]uuid.UUID, error) {
	allPrincipalIDs, err := db.GetAllParentIDs(ctx, m.pool, principalID)
	if err != nil {
		return nil, fmt.Errorf("principals: effective scope ids: %w", err)
	}
	scopeIDs, err := db.New(m.pool).GetScopesForPrincipals(ctx, allPrincipalIDs)
	if err != nil {
		return nil, fmt.Errorf("principals: effective scope ids query: %w", err)
	}
	return scopeIDs, nil
}

// IsSystemAdmin returns true if the principal has the is_system_admin flag set.
// Returns false (not an error) when the principal does not exist.
func (m *MembershipStore) IsSystemAdmin(ctx context.Context, principalID uuid.UUID) (bool, error) {
	isAdmin, err := db.New(m.pool).GetPrincipalIsSystemAdmin(ctx, principalID)
	if errors.Is(err, pgx.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("principals: is system admin: %w", err)
	}
	return isAdmin, nil
}

// IsScopeAdmin returns true if principalID has role="admin" in the given scope or any ancestor scope.
func (m *MembershipStore) IsScopeAdmin(ctx context.Context, principalID, scopeID uuid.UUID) (bool, error) {
	isSystemAdmin, err := m.IsSystemAdmin(ctx, principalID)
	if err != nil {
		return false, fmt.Errorf("principals: is scope admin system check: %w", err)
	}
	if isSystemAdmin {
		return true, nil
	}

	ancestorScopeIDs, err := db.GetAncestorScopeIDs(ctx, m.pool, scopeID)
	if err != nil {
		return false, fmt.Errorf("principals: is scope admin: %w", err)
	}

	exists, err := db.New(m.pool).IsScopeAdmin(ctx, db.IsScopeAdminParams{
		Column1:     ancestorScopeIDs,
		PrincipalID: principalID,
	})
	if err != nil {
		return false, fmt.Errorf("principals: is scope admin query: %w", err)
	}
	return exists, nil
}

// IsPrincipalAdmin returns true if principalID has role="admin" on targetPrincipalID
// or any ancestor principal of targetPrincipalID.
func (m *MembershipStore) IsPrincipalAdmin(ctx context.Context, principalID, targetPrincipalID uuid.UUID) (bool, error) {
	isSystemAdmin, err := m.IsSystemAdmin(ctx, principalID)
	if err != nil {
		return false, fmt.Errorf("principals: is principal admin system check: %w", err)
	}
	if isSystemAdmin {
		return true, nil
	}

	ancestorPrincipalIDs, err := db.GetAllParentIDs(ctx, m.pool, targetPrincipalID)
	if err != nil {
		return false, fmt.Errorf("principals: is principal admin: %w", err)
	}

	exists, err := db.New(m.pool).IsPrincipalAdmin(ctx, db.IsPrincipalAdminParams{
		MemberID: principalID,
		Column2:  ancestorPrincipalIDs,
	})
	if err != nil {
		return false, fmt.Errorf("principals: is principal admin query: %w", err)
	}
	return exists, nil
}

// HasAnyAdminRole returns true if principalID holds at least one admin membership.
func (m *MembershipStore) HasAnyAdminRole(ctx context.Context, principalID uuid.UUID) (bool, error) {
	isSystemAdmin, err := m.IsSystemAdmin(ctx, principalID)
	if err != nil {
		return false, fmt.Errorf("principals: has any admin role system check: %w", err)
	}
	if isSystemAdmin {
		return true, nil
	}

	exists, err := db.New(m.pool).HasAnyAdminMembership(ctx, principalID)
	if err != nil {
		return false, fmt.Errorf("principals: has any admin role query: %w", err)
	}
	return exists, nil
}
