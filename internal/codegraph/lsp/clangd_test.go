package lsp

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

func TestClangdClient_Imports_ParsesIncludeDirectives(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	file := filepath.Join(dir, "main.cpp")
	src := `#include <vector>
#include "local/header.hpp"

int main() { return 0; }
`
	if err := os.WriteFile(file, []byte(src), 0o600); err != nil {
		t.Fatalf("write temp source: %v", err)
	}

	c := &ClangdClient{}
	imports, err := c.Imports(context.Background(), file)
	if err != nil {
		t.Fatalf("Imports: %v", err)
	}
	if len(imports) != 2 {
		t.Fatalf("len(imports) = %d, want 2", len(imports))
	}
	if imports[0].Path != "vector" || !imports[0].IsStdlib {
		t.Fatalf("imports[0] = %+v, want stdlib vector include", imports[0])
	}
	if imports[1].Path != "local/header.hpp" || imports[1].IsStdlib {
		t.Fatalf("imports[1] = %+v, want local header include", imports[1])
	}
}
