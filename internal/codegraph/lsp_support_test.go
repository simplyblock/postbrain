package codegraph

import "testing"

func TestLSPSupportsFile_AliasExtension(t *testing.T) {
	t.Parallel()

	client := &stubLSPClient{
		languages: map[string]int{
			".c":   100,
			".cpp": 90,
		},
	}
	if !lspSupportsFile(client, "src/main.cpp") {
		t.Fatal("expected .cpp alias extension to be supported")
	}
}

func TestLSPSupportsExt_NormalizesCaseAndDot(t *testing.T) {
	t.Parallel()

	client := &stubLSPClient{
		languages: map[string]int{
			".ts": 100,
		},
	}
	if !lspSupportsExt(client, "TS") {
		t.Fatal("expected extension normalization to support TS")
	}
}
