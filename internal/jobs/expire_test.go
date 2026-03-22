package jobs

import (
	"context"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"
)

// TestExpireWorkingMemory_Signature is a compile-time check that the function
// has the correct signature. Actual DB behaviour is covered by integration tests.
func TestExpireWorkingMemory_Signature(t *testing.T) {
	// Verify the function signature compiles correctly.
	var _ func(context.Context, *pgxpool.Pool) (int64, error) = ExpireWorkingMemory
}
