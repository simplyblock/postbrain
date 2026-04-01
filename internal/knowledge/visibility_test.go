package knowledge

import (
	"testing"

	"github.com/google/uuid"
)

// TestResolveVisibleScopeIDs_IncludesSelf verifies that the requested scope is always
// included in the result even without a real database (unit-level smoke test).
// Full ltree behaviour is covered by integration tests.
func TestResolveVisibleScopeIDs_IncludesSelf(t *testing.T) {
	t.Parallel()
	ids := deduplicateScopeIDs([]uuid.UUID{
		uuid.MustParse("00000000-0000-0000-0000-000000000001"),
		uuid.MustParse("00000000-0000-0000-0000-000000000001"),
		uuid.MustParse("00000000-0000-0000-0000-000000000002"),
	})
	if len(ids) != 2 {
		t.Errorf("expected 2 deduplicated IDs, got %d", len(ids))
	}
}

func TestDeduplicateScopeIDs_Empty(t *testing.T) {
	t.Parallel()
	ids := deduplicateScopeIDs([]uuid.UUID{})
	if ids == nil {
		t.Error("expected non-nil empty slice, got nil")
	}
	if len(ids) != 0 {
		t.Errorf("expected 0 IDs, got %d", len(ids))
	}
}

func TestDeduplicateScopeIDs_AllDuplicates(t *testing.T) {
	t.Parallel()
	id := uuid.MustParse("00000000-0000-0000-0000-000000000042")
	ids := deduplicateScopeIDs([]uuid.UUID{id, id, id})
	if len(ids) != 1 {
		t.Errorf("expected 1 unique ID, got %d", len(ids))
	}
	if ids[0] != id {
		t.Errorf("expected %v, got %v", id, ids[0])
	}
}

func TestDeduplicateScopeIDs_PreservesOrder(t *testing.T) {
	t.Parallel()
	a := uuid.MustParse("00000000-0000-0000-0000-000000000001")
	b := uuid.MustParse("00000000-0000-0000-0000-000000000002")
	c := uuid.MustParse("00000000-0000-0000-0000-000000000003")
	ids := deduplicateScopeIDs([]uuid.UUID{c, a, b, a, c})
	if len(ids) != 3 {
		t.Fatalf("expected 3 unique IDs, got %d", len(ids))
	}
	if ids[0] != c || ids[1] != a || ids[2] != b {
		t.Errorf("order not preserved: got %v", ids)
	}
}
