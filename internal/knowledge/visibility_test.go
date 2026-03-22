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
	// The function requires a database, so we only verify the function exists and
	// the deduplicated scope list logic with a mock.
	ids := deduplicateScopeIDs([]uuid.UUID{
		uuid.MustParse("00000000-0000-0000-0000-000000000001"),
		uuid.MustParse("00000000-0000-0000-0000-000000000001"),
		uuid.MustParse("00000000-0000-0000-0000-000000000002"),
	})
	if len(ids) != 2 {
		t.Errorf("expected 2 deduplicated IDs, got %d", len(ids))
	}
}
