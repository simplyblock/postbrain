package codegraph

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/simplyblock/postbrain/internal/codegraph/lsp"
)

func TestLSPRegistry_EmptyRootDirs_Disabled(t *testing.T) {
	called := false
	prev := newLSPClientForExt
	newLSPClientForExt = func(string, string, time.Duration, lsp.ClientOptions) (lsp.Client, error) {
		called = true
		return nil, nil
	}
	t.Cleanup(func() { newLSPClientForExt = prev })

	got := newLSPRegistry(IndexOptions{})
	if len(got) != 0 {
		t.Fatalf("len(clients) = %d, want 0", len(got))
	}
	if called {
		t.Fatal("did not expect factory call when all root dirs are empty")
	}
}

func TestLSPRegistry_DoesNotCreateClientsUntilExtensionMatch(t *testing.T) {
	prev := newLSPClientForExt
	var exts []string
	newLSPClientForExt = func(ext, rootDir string, timeout time.Duration, opts lsp.ClientOptions) (lsp.Client, error) {
		exts = append(exts, ext)
		return &stubLSPClient{
			languages: map[string]int{ext: 100},
		}, nil
	}
	t.Cleanup(func() { newLSPClientForExt = prev })

	registry := newLSPRegistry(IndexOptions{
		GoLSPRootDir:         "/tmp/go",
		GoLSPTimeout:         1 * time.Second,
		TypeScriptLSPRootDir: "/tmp/ts",
		TypeScriptLSPTimeout: 2 * time.Second,
		ClangdLSPRootDir:     "/tmp/c",
		ClangdLSPTimeout:     3 * time.Second,
		PythonLSPRootDir:     "/tmp/py",
		PythonLSPTimeout:     3500 * time.Millisecond,
		MarkdownLSPRootDir:   "/tmp/md",
		MarkdownLSPTimeout:   4 * time.Second,
	})
	if len(registry) != 5 {
		t.Fatalf("len(registry) = %d, want 5", len(registry))
	}
	if len(exts) != 0 {
		t.Fatalf("len(created) = %d, want 0 before first file match", len(exts))
	}

	client, root := lspClientForFile(context.Background(), "web/app.tsx", registry)
	if client == nil {
		t.Fatal("expected TypeScript client to be created lazily")
	}
	if root != "/tmp/ts" {
		t.Fatalf("root = %q, want %q", root, "/tmp/ts")
	}
	if len(exts) != 1 || exts[0] != ".ts" {
		t.Fatalf("created exts = %v, want [.ts]", exts)
	}
}

func TestLSPRegistry_FactoryError_DoesNotBlockOtherClients(t *testing.T) {
	prev := newLSPClientForExt
	newLSPClientForExt = func(ext, rootDir string, timeout time.Duration, opts lsp.ClientOptions) (lsp.Client, error) {
		if ext == ".go" {
			return nil, errors.New("gopls not found")
		}
		return &stubLSPClient{languages: map[string]int{ext: 100}}, nil
	}
	t.Cleanup(func() { newLSPClientForExt = prev })

	registry := newLSPRegistry(IndexOptions{
		GoLSPRootDir:         "/tmp/go",
		TypeScriptLSPRootDir: "/tmp/ts",
	})
	if len(registry) != 2 {
		t.Fatalf("len(registry) = %d, want 2", len(registry))
	}

	goClient, _ := lspClientForFile(context.Background(), "svc/main.go", registry)
	if goClient != nil {
		t.Fatal("expected nil Go client when factory creation fails")
	}
	tsClient, _ := lspClientForFile(context.Background(), "web/app.ts", registry)
	if tsClient == nil {
		t.Fatal("expected surviving TypeScript client")
	}
}

func TestLSPRegistry_Python_Enabled_UsesPyright(t *testing.T) {
	prev := newLSPClientForExt
	newLSPClientForExt = func(ext, rootDir string, timeout time.Duration, opts lsp.ClientOptions) (lsp.Client, error) {
		if ext != ".py" {
			t.Fatalf("ext = %q, want %q", ext, ".py")
		}
		if rootDir != "/tmp/pyrepo" {
			t.Fatalf("rootDir = %q, want %q", rootDir, "/tmp/pyrepo")
		}
		if timeout != 2*time.Second {
			t.Fatalf("timeout = %v, want %v", timeout, 2*time.Second)
		}
		if opts.UseTSGo {
			t.Fatal("UseTSGo must be false for pyright")
		}
		return &stubLSPClient{languages: map[string]int{".py": 100}}, nil
	}
	t.Cleanup(func() { newLSPClientForExt = prev })

	registry := newLSPRegistry(IndexOptions{
		PythonLSPRootDir: "/tmp/pyrepo",
		PythonLSPTimeout: 2 * time.Second,
	})
	client, root := lspClientForFile(context.Background(), "src/app.py", registry)
	if client == nil {
		t.Fatal("expected Python client")
	}
	if root != "/tmp/pyrepo" {
		t.Fatalf("root = %q, want %q", root, "/tmp/pyrepo")
	}
}

func TestLSPClientForFile_SelectsHighestPriorityClient(t *testing.T) {
	t.Parallel()

	low := &stubLSPClient{languages: map[string]int{".ts": 80}}
	high := &stubLSPClient{languages: map[string]int{".ts": 100}}
	client, root := lspClientForFile(context.Background(), "web/app.ts", []lspSelection{
		{client: low, rootDir: "/tmp/low"},
		{client: high, rootDir: "/tmp/high"},
	})
	if client != high {
		t.Fatal("expected highest-priority client")
	}
	if root != "/tmp/high" {
		t.Fatalf("root = %q, want %q", root, "/tmp/high")
	}
}

func TestLSPClientForFile_SupportsAliasExtension(t *testing.T) {
	t.Parallel()

	clang := &stubLSPClient{
		languages: map[string]int{
			".c":   100,
			".cpp": 90,
		},
	}
	client, root := lspClientForFile(context.Background(), "src/main.cpp", []lspSelection{
		{client: clang, rootDir: "/tmp/cpp"},
	})
	if client == nil {
		t.Fatal("expected client for .cpp alias extension")
	}
	if root != "/tmp/cpp" {
		t.Fatalf("root = %q, want %q", root, "/tmp/cpp")
	}
}
