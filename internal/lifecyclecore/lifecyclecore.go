// Package lifecyclecore provides shared primitives for knowledge and skill
// lifecycle state machines: the MembershipChecker interface, the IsEffectiveAdmin
// helper, and the EndorseResult value type.
package lifecyclecore

import (
	"context"

	"github.com/google/uuid"
)

// MembershipChecker can determine admin status for a principal.
type MembershipChecker interface {
	IsScopeAdmin(ctx context.Context, principalID, scopeID uuid.UUID) (bool, error)
	IsSystemAdmin(ctx context.Context, principalID uuid.UUID) (bool, error)
}

// IsEffectiveAdmin returns true if principalID is either a system admin or a
// scope admin on scopeID. System admin is checked first; a nil checker always
// returns false.
func IsEffectiveAdmin(ctx context.Context, m MembershipChecker, principalID, scopeID uuid.UUID) (bool, error) {
	if m == nil {
		return false, nil
	}
	sysAdmin, err := m.IsSystemAdmin(ctx, principalID)
	if err != nil || sysAdmin {
		return sysAdmin, err
	}
	return m.IsScopeAdmin(ctx, principalID, scopeID)
}

// EndorseResult carries the outcome of an Endorse call.
type EndorseResult struct {
	EndorsementCount int
	Status           string
	AutoPublished    bool
}
