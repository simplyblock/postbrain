package knowledge

import (
	"context"
	"testing"

	"github.com/google/uuid"
)

func TestCollectionStore_Create_InvalidVisibility(t *testing.T) {
	t.Parallel()
	cs := NewCollectionStore(nil)
	_, err := cs.Create(context.Background(), uuid.New(), uuid.New(), "my-slug", "My Collection", "invalid-visibility", nil)
	if err == nil {
		t.Fatal("expected error for invalid visibility, got nil")
	}
}

func TestCollectionStore_Create_ValidVisibility_NoValidationError(t *testing.T) {
	t.Parallel()
	validValues := []string{"private", "project", "team", "department", "company"}
	for _, v := range validValues {
		v := v
		t.Run(v, func(t *testing.T) {
			t.Parallel()
			// Verify that valid visibility values do not trigger the validation error.
			if _, ok := validVisibilities[v]; !ok {
				t.Errorf("expected %q to be a valid visibility", v)
			}
		})
	}
}
