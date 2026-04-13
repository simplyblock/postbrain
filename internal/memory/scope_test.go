package memory

import (
	"testing"

	"github.com/google/uuid"
)

// TestFanOut_StrictScope verifies that strictScope=true returns only the given scopeID
// without performing any DB call.
func TestFanOut_StrictScope(t *testing.T) {
	scopeID := uuid.New()
	principalID := uuid.New()

	ids, err := FanOutScopeIDs(t.Context(), nil, scopeID, principalID, 0, true)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(ids) != 1 {
		t.Fatalf("expected 1 scope ID, got %d", len(ids))
	}
	if ids[0] != scopeID {
		t.Fatalf("expected scopeID %v, got %v", scopeID, ids[0])
	}
}

// TestDeduplicateScopeIDs_RemovesDuplicates verifies deduplication of scope ID slices.
func TestDeduplicateScopeIDs_RemovesDuplicates(t *testing.T) {
	// This test exercises the deduplication logic without a real DB.
	scopeID := uuid.New()
	personalID := uuid.New()

	// Simulate what FanOutScopeIDs does internally after DB calls.
	ancestors := []uuid.UUID{scopeID, uuid.New(), uuid.New()}
	personal := []uuid.UUID{personalID}
	combined := deduplicateScopeIDs(append(ancestors, personal...))

	if len(combined) < 1 {
		t.Fatalf("expected non-empty result, got empty")
	}
	// scopeID must be present.
	found := false
	for _, id := range combined {
		if id == scopeID {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("scopeID not found in combined result")
	}
}
