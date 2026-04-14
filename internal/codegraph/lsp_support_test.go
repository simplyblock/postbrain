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

func TestLSPRootDirForClient_PrefersHighestPrioritySupportedExtension(t *testing.T) {
	t.Parallel()

	client := &stubLSPClient{
		languages: map[string]int{
			".js":  90,
			".ts":  100,
			".tsx": 95,
		},
	}
	got := lspRootDirForClient(IndexOptions{
		TypeScriptLSPRootDir: "/tmp/tsrepo",
	}, client)
	if got != "/tmp/tsrepo" {
		t.Fatalf("root dir = %q, want %q", got, "/tmp/tsrepo")
	}
}
