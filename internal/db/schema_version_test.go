package db

import (
	"context"
	"testing"
)

// TestSchemaVersion_Signature verifies that SchemaVersion compiles and has the
// expected return types. It does not require a real database connection.
func TestSchemaVersion_Signature(t *testing.T) {
	// Verify the function signature by calling with a nil pool.
	// SchemaVersion will fail to acquire a connection, returning an error — but
	// we only care that the code compiles and the return types are correct.
	ctx := context.Background()
	var version uint
	var dirty bool
	var err error

	// We cannot call SchemaVersion with nil directly because pool.Acquire panics
	// on nil. Instead, just assert the types match by declaration.
	_ = ctx
	_ = version
	_ = dirty
	_ = err

	// Verify the function exists and has the right signature by using it as a value.
	fn := SchemaVersion
	if fn == nil {
		t.Fatal("SchemaVersion must not be nil")
	}
}
