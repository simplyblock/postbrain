package scopeutil_test

import (
	"strings"
	"testing"

	"github.com/simplyblock/postbrain/internal/scopeutil"
)

func TestParseScopeString(t *testing.T) {
	tests := []struct {
		input      string
		wantKind   string
		wantExtID  string
		wantErrMsg string
	}{
		// valid
		{"project:myorg/api", "project", "myorg/api", ""},
		{"user:alice", "user", "alice", ""},
		// colon in external_id is fine — only the first colon is the separator
		{"kind:foo:bar", "kind", "foo:bar", ""},
		// whitespace is trimmed
		{"  project : myorg/api  ", "project", "myorg/api", ""},

		// invalid
		{"", "", "", "empty scope string"},
		{"   ", "", "", "empty scope string"},
		{"nocolon", "", "", "missing ':'"},
		{":external_id", "", "", "empty kind"},
		{"kind:", "", "", "empty external_id"},
		{"  :  ", "", "", "empty kind"},
	}

	for _, tc := range tests {
		t.Run(tc.input, func(t *testing.T) {
			kind, extID, err := scopeutil.ParseScopeString(tc.input)
			if tc.wantErrMsg == "" {
				if err != nil {
					t.Fatalf("unexpected error: %v", err)
				}
				if kind != tc.wantKind {
					t.Errorf("kind: got %q, want %q", kind, tc.wantKind)
				}
				if extID != tc.wantExtID {
					t.Errorf("externalID: got %q, want %q", extID, tc.wantExtID)
				}
			} else {
				if err == nil {
					t.Fatalf("expected error containing %q, got nil (kind=%q extID=%q)", tc.wantErrMsg, kind, extID)
				}
				if !strings.Contains(err.Error(), tc.wantErrMsg) {
					t.Errorf("error %q does not contain %q", err.Error(), tc.wantErrMsg)
				}
			}
		})
	}
}
