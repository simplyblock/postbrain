package db

import (
	"context"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"
)

// TestSchemaVersion_Signature verifies that SchemaVersion compiles and has the
// expected return types. It does not require a real database connection.
func TestSchemaVersion_Signature(t *testing.T) {
	// Compile-time signature enforcement.
	requireSchemaVersionSignature(SchemaVersion)
}

func requireSchemaVersionSignature(fn func(context.Context, *pgxpool.Pool) (uint, bool, error)) {
	_ = fn
}
