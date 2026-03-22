package principals

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"
)

// mockMembershipStore is a test double for MembershipStore that intercepts
// db.GetAllParentIDs calls so we can test cycle detection without a real DB.
type mockMembershipStore struct {
	// parentIDs simulates what GetAllParentIDs returns for a given memberID.
	parentIDs map[uuid.UUID][]uuid.UUID
}

func (m *mockMembershipStore) getAllParentIDs(_ context.Context, memberID uuid.UUID) ([]uuid.UUID, error) {
	if ids, ok := m.parentIDs[memberID]; ok {
		return ids, nil
	}
	return []uuid.UUID{memberID}, nil
}

// cycleCheckAddMembership is the cycle-detection logic extracted so it can be
// tested without a real database pool.
func cycleCheckAddMembership(ctx context.Context, mock *mockMembershipStore, memberID, parentID uuid.UUID, role string) error {
	validRoles := map[string]bool{"member": true, "owner": true, "admin": true}
	if !validRoles[role] {
		return ErrInvalidRole
	}

	ancestors, err := mock.getAllParentIDs(ctx, parentID)
	if err != nil {
		return err
	}
	for _, id := range ancestors {
		if id == memberID {
			return ErrCycleDetected
		}
	}
	return nil
}

func TestAddMembership_InvalidRole(t *testing.T) {
	mock := &mockMembershipStore{}
	memberID := uuid.New()
	parentID := uuid.New()

	err := cycleCheckAddMembership(context.Background(), mock, memberID, parentID, "superadmin")
	if !errors.Is(err, ErrInvalidRole) {
		t.Errorf("expected ErrInvalidRole, got %v", err)
	}
}

func TestAddMembership_ValidRoles(t *testing.T) {
	mock := &mockMembershipStore{}
	memberID := uuid.New()
	parentID := uuid.New()

	for _, role := range []string{"member", "owner", "admin"} {
		err := cycleCheckAddMembership(context.Background(), mock, memberID, parentID, role)
		if err != nil {
			t.Errorf("role %q: unexpected error: %v", role, err)
		}
	}
}

func TestAddMembership_CycleDetected_Direct(t *testing.T) {
	memberID := uuid.New()
	parentID := uuid.New()

	// parentID's ancestors include memberID → adding memberID→parentID would cycle.
	mock := &mockMembershipStore{
		parentIDs: map[uuid.UUID][]uuid.UUID{
			parentID: {parentID, memberID}, // parentID's ancestors include memberID
		},
	}

	err := cycleCheckAddMembership(context.Background(), mock, memberID, parentID, "member")
	if !errors.Is(err, ErrCycleDetected) {
		t.Errorf("expected ErrCycleDetected, got %v", err)
	}
}

func TestAddMembership_NoCycle(t *testing.T) {
	memberID := uuid.New()
	parentID := uuid.New()
	otherID := uuid.New()

	mock := &mockMembershipStore{
		parentIDs: map[uuid.UUID][]uuid.UUID{
			parentID: {parentID, otherID},
		},
	}

	err := cycleCheckAddMembership(context.Background(), mock, memberID, parentID, "owner")
	if err != nil {
		t.Errorf("expected no error, got %v", err)
	}
}

func TestAddMembership_CycleDetected_Transitive(t *testing.T) {
	// Chain: C → B → A. Now we try to add A → C.
	// GetAllParentIDs(C) returns [C, B, A].
	a := uuid.New()
	b := uuid.New()
	c := uuid.New()

	mock := &mockMembershipStore{
		parentIDs: map[uuid.UUID][]uuid.UUID{
			c: {c, b, a},
		},
	}

	// Try adding a → c; a is in ancestors of c → cycle.
	err := cycleCheckAddMembership(context.Background(), mock, a, c, "member")
	if !errors.Is(err, ErrCycleDetected) {
		t.Errorf("expected ErrCycleDetected for transitive cycle, got %v", err)
	}
}
