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
	newLSPClientForExt = func(string, string, time.Duration, lsp.ClientOptions) (lsp.Client, error) {
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
	want := &stubLSPClient{languages: map[string]int{".go": 100}}
	prev := newLSPClientForExt
	newLSPClientForExt = func(ext, rootDir string, timeout time.Duration, opts lsp.ClientOptions) (lsp.Client, error) {
		if ext != ".go" {
			t.Fatalf("ext = %q, want %q", ext, ".go")
		}
		if rootDir != "/tmp/repo" {
			t.Fatalf("rootDir = %q, want %q", rootDir, "/tmp/repo")
		}
		if timeout != 3*time.Second {
			t.Fatalf("timeout = %v, want %v", timeout, 3*time.Second)
		}
		if opts.UseTSGo {
			t.Fatal("UseTSGo must be false for Go LSP")
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
	newLSPClientForExt = func(string, string, time.Duration, lsp.ClientOptions) (lsp.Client, error) {
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
	newLSPClientForExt = func(string, string, time.Duration, lsp.ClientOptions) (lsp.Client, error) {
		return nil, nil // unsupported extension
	}
	t.Cleanup(func() { newLSPClientForExt = prev })

	got := lspClientForIndex(context.Background(), IndexOptions{GoLSPRootDir: "/tmp/repo"})
	if got != nil {
		t.Fatal("expected nil client when factory returns nil")
	}
}

func TestLSPClientForIndex_TypeScript_Enabled_DefaultBackend(t *testing.T) {
	want := &stubLSPClient{languages: map[string]int{".ts": 100}}
	prev := newLSPClientForExt
	newLSPClientForExt = func(ext, rootDir string, timeout time.Duration, opts lsp.ClientOptions) (lsp.Client, error) {
		if ext != ".ts" {
			t.Fatalf("ext = %q, want %q", ext, ".ts")
		}
		if rootDir != "/tmp/tsrepo" {
			t.Fatalf("rootDir = %q, want %q", rootDir, "/tmp/tsrepo")
		}
		if timeout != 4*time.Second {
			t.Fatalf("timeout = %v, want %v", timeout, 4*time.Second)
		}
		if opts.UseTSGo {
			t.Fatal("UseTSGo must be false by default")
		}
		return want, nil
	}
	t.Cleanup(func() { newLSPClientForExt = prev })

	got := lspClientForIndex(context.Background(), IndexOptions{
		TypeScriptLSPRootDir: "/tmp/tsrepo",
		TypeScriptLSPTimeout: 4 * time.Second,
	})
	if got != want {
		t.Fatal("expected returned TypeScript client from factory")
	}
}

func TestLSPClientForIndex_TypeScript_Enabled_TSGoFlag(t *testing.T) {
	want := &stubLSPClient{languages: map[string]int{".ts": 100}}
	prev := newLSPClientForExt
	newLSPClientForExt = func(ext, rootDir string, timeout time.Duration, opts lsp.ClientOptions) (lsp.Client, error) {
		if ext != ".ts" {
			t.Fatalf("ext = %q, want %q", ext, ".ts")
		}
		if !opts.UseTSGo {
			t.Fatal("UseTSGo must be true when TypeScriptLSPUseTSGo is set")
		}
		return want, nil
	}
	t.Cleanup(func() { newLSPClientForExt = prev })

	got := lspClientForIndex(context.Background(), IndexOptions{
		TypeScriptLSPRootDir: "/tmp/tsrepo",
		TypeScriptLSPUseTSGo: true,
	})
	if got != want {
		t.Fatal("expected returned TypeScript client from factory")
	}
}

func TestLSPClientForIndex_Clangd_Enabled(t *testing.T) {
	want := &stubLSPClient{languages: map[string]int{".c": 100}}
	prev := newLSPClientForExt
	newLSPClientForExt = func(ext, rootDir string, timeout time.Duration, opts lsp.ClientOptions) (lsp.Client, error) {
		if ext != ".c" {
			t.Fatalf("ext = %q, want %q", ext, ".c")
		}
		if rootDir != "/tmp/crepo" {
			t.Fatalf("rootDir = %q, want %q", rootDir, "/tmp/crepo")
		}
		if timeout != 2*time.Second {
			t.Fatalf("timeout = %v, want %v", timeout, 2*time.Second)
		}
		if opts.UseTSGo {
			t.Fatal("UseTSGo must be false for clangd")
		}
		return want, nil
	}
	t.Cleanup(func() { newLSPClientForExt = prev })

	got := lspClientForIndex(context.Background(), IndexOptions{
		ClangdLSPRootDir: "/tmp/crepo",
		ClangdLSPTimeout: 2 * time.Second,
	})
	if got != want {
		t.Fatal("expected returned clangd client from factory")
	}
}

func TestLSPClientForIndex_Markdown_Enabled(t *testing.T) {
	want := &stubLSPClient{languages: map[string]int{".md": 100}}
	prev := newLSPClientForExt
	newLSPClientForExt = func(ext, rootDir string, timeout time.Duration, opts lsp.ClientOptions) (lsp.Client, error) {
		if ext != ".md" {
			t.Fatalf("ext = %q, want %q", ext, ".md")
		}
		if rootDir != "/tmp/mdrepo" {
			t.Fatalf("rootDir = %q, want %q", rootDir, "/tmp/mdrepo")
		}
		if timeout != 1500*time.Millisecond {
			t.Fatalf("timeout = %v, want %v", timeout, 1500*time.Millisecond)
		}
		if opts.UseTSGo {
			t.Fatal("UseTSGo must be false for marksman")
		}
		return want, nil
	}
	t.Cleanup(func() { newLSPClientForExt = prev })

	got := lspClientForIndex(context.Background(), IndexOptions{
		MarkdownLSPRootDir: "/tmp/mdrepo",
		MarkdownLSPTimeout: 1500 * time.Millisecond,
	})
	if got != want {
		t.Fatal("expected returned markdown client from factory")
	}
}
