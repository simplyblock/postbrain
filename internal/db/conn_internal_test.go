package db

import (
	"strings"
	"testing"
)

func TestDefaultSearchPathSQL_IncludesAGECatalog(t *testing.T) {
	if !strings.Contains(defaultSearchPathSQL, "ag_catalog") {
		t.Fatalf("default search_path must include ag_catalog for AGE compatibility: %q", defaultSearchPathSQL)
	}
	if !strings.Contains(defaultSearchPathSQL, "\"$user\"") {
		t.Fatalf("default search_path must include $user schema: %q", defaultSearchPathSQL)
	}
	if !strings.Contains(defaultSearchPathSQL, "public") {
		t.Fatalf("default search_path must include public schema: %q", defaultSearchPathSQL)
	}
}
