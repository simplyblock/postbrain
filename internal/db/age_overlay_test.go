package db

import (
	"strings"
	"testing"
)

func TestEnsureAGEAccessProbeSQL_IsReadOnly(t *testing.T) {
	if strings.Contains(ensureAGEAccessProbeSQL, "CREATE (") {
		t.Fatalf("AGE startup probe must be read-only: %q", ensureAGEAccessProbeSQL)
	}
	if !strings.Contains(ensureAGEAccessProbeSQL, "RETURN 1") {
		t.Fatalf("AGE startup probe must execute a minimal query: %q", ensureAGEAccessProbeSQL)
	}
}
