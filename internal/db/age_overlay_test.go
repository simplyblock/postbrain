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

func TestEnsureAGEPrivilegesSQL_DoesNotGrantToPublic(t *testing.T) {
	if strings.Contains(ensureAGEPrivilegesSQL, " TO PUBLIC") {
		t.Fatalf("AGE privilege bootstrap must not grant to PUBLIC: %q", ensureAGEPrivilegesSQL)
	}
	if !strings.Contains(ensureAGEPrivilegesSQL, "quote_ident(current_user)") {
		t.Fatalf("AGE privilege bootstrap should target current_user role: %q", ensureAGEPrivilegesSQL)
	}
}
