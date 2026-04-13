package codegraph

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/simplyblock/postbrain/internal/codegraph/lsp"
)

func TestLSPClientForIndex_EmptyRootDir_Disabled(t *testing.T) {
	called := false
	prev := newLSPClientForExt
	newLSPClientForExt = func(string, string, time.Duration) (lsp.Client, error) {
		called = true
		return nil, nil
	}
	t.Cleanup(func() { newLSPClientForExt = prev })

	got := lspClientForIndex(context.Background(), IndexOptions{})
	if got != nil {
		t.Fatal("expected nil client when GoLSPRootDir is empty")
	}
	if called {
		t.Fatal("did not expect factory call when GoLSPRootDir is empty")
	}
}

func TestLSPClientForIndex_Enabled_ReturnsClient(t *testing.T) {
	want := &stubLSPClient{lang: ".go"}
	prev := newLSPClientForExt
	newLSPClientForExt = func(ext, rootDir string, timeout time.Duration) (lsp.Client, error) {
		if ext != ".go" {
			t.Fatalf("ext = %q, want %q", ext, ".go")
		}
		if rootDir != "/tmp/repo" {
			t.Fatalf("rootDir = %q, want %q", rootDir, "/tmp/repo")
		}
		if timeout != 3*time.Second {
			t.Fatalf("timeout = %v, want %v", timeout, 3*time.Second)
		}
		return want, nil
	}
	t.Cleanup(func() { newLSPClientForExt = prev })

	got := lspClientForIndex(context.Background(), IndexOptions{
		GoLSPRootDir: "/tmp/repo",
		GoLSPTimeout: 3 * time.Second,
	})
	if got != want {
		t.Fatal("expected returned client from factory")
	}
}

func TestLSPClientForIndex_FactoryError_FallsBackToNil(t *testing.T) {
	prev := newLSPClientForExt
	newLSPClientForExt = func(string, string, time.Duration) (lsp.Client, error) {
		return nil, errors.New("gopls not found")
	}
	t.Cleanup(func() { newLSPClientForExt = prev })

	got := lspClientForIndex(context.Background(), IndexOptions{GoLSPRootDir: "/tmp/repo"})
	if got != nil {
		t.Fatal("expected nil client on factory error")
	}
}

func TestLSPClientForIndex_NilClient_ReturnsNil(t *testing.T) {
	prev := newLSPClientForExt
	newLSPClientForExt = func(string, string, time.Duration) (lsp.Client, error) {
		return nil, nil // unsupported extension
	}
	t.Cleanup(func() { newLSPClientForExt = prev })

	got := lspClientForIndex(context.Background(), IndexOptions{GoLSPRootDir: "/tmp/repo"})
	if got != nil {
		t.Fatal("expected nil client when factory returns nil")
	}
}
