package codegraph

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/simplyblock/postbrain/internal/codegraph/lsp"
)

func TestLSPResolverForIndex_EmptyRootDir_Disabled(t *testing.T) {
	called := false
	prev := newLSPClientForExt
	newLSPClientForExt = func(string, string, time.Duration) (lsp.Client, error) {
		called = true
		return nil, nil
	}
	t.Cleanup(func() { newLSPClientForExt = prev })

	got := lspResolverForIndex(context.Background(), IndexOptions{})
	if got != nil {
		t.Fatal("expected nil resolver when GoLSPRootDir is empty")
	}
	if called {
		t.Fatal("did not expect factory call when GoLSPRootDir is empty")
	}
}

func TestLSPResolverForIndex_Enabled_ReturnsAdapter(t *testing.T) {
	want := &fakeClient{}
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

	got := lspResolverForIndex(context.Background(), IndexOptions{
		GoLSPRootDir: "/tmp/repo",
		GoLSPTimeout: 3 * time.Second,
	})
	if got == nil {
		t.Fatal("expected non-nil resolver")
	}
	adapter, ok := got.(*lspClientAdapter)
	if !ok {
		t.Fatalf("got %T, want *lspClientAdapter", got)
	}
	if adapter.client != want {
		t.Fatal("adapter wraps wrong client")
	}
	if adapter.rootDir != "/tmp/repo" {
		t.Fatalf("adapter.rootDir = %q, want %q", adapter.rootDir, "/tmp/repo")
	}
}

func TestLSPResolverForIndex_FactoryError_FallsBackToNil(t *testing.T) {
	prev := newLSPClientForExt
	newLSPClientForExt = func(string, string, time.Duration) (lsp.Client, error) {
		return nil, errors.New("gopls not found")
	}
	t.Cleanup(func() { newLSPClientForExt = prev })

	got := lspResolverForIndex(context.Background(), IndexOptions{GoLSPRootDir: "/tmp/repo"})
	if got != nil {
		t.Fatal("expected nil resolver on factory error")
	}
}

func TestLSPResolverForIndex_NilClient_FallsBackToNil(t *testing.T) {
	prev := newLSPClientForExt
	newLSPClientForExt = func(string, string, time.Duration) (lsp.Client, error) {
		return nil, nil // unsupported extension
	}
	t.Cleanup(func() { newLSPClientForExt = prev })

	got := lspResolverForIndex(context.Background(), IndexOptions{GoLSPRootDir: "/tmp/repo"})
	if got != nil {
		t.Fatal("expected nil resolver when factory returns nil client")
	}
}

// fakeClient is a minimal lsp.Client stub used only to satisfy the interface
// in factory tests.
type fakeClient struct{}

func (f *fakeClient) Language() string { return ".go" }
func (f *fakeClient) Definition(context.Context, string, lsp.Position) ([]lsp.Location, error) {
	return nil, nil
}
func (f *fakeClient) References(context.Context, string, lsp.Position, bool) ([]lsp.Location, error) {
	return nil, nil
}
func (f *fakeClient) IncomingCalls(context.Context, string, lsp.Position) ([]lsp.CallSite, error) {
	return nil, nil
}
func (f *fakeClient) OutgoingCalls(context.Context, string, lsp.Position) ([]lsp.CallSite, error) {
	return nil, nil
}
func (f *fakeClient) DocumentSymbols(context.Context, string) ([]lsp.Symbol, error) { return nil, nil }
func (f *fakeClient) WorkspaceSymbols(context.Context, string) ([]lsp.Symbol, error) {
	return nil, nil
}
func (f *fakeClient) Imports(context.Context, string) ([]lsp.Import, error)      { return nil, nil }
func (f *fakeClient) Hover(context.Context, string, lsp.Position) (string, error) { return "", nil }
func (f *fakeClient) CanonicalName(context.Context, string, lsp.Position) (string, error) {
	return "", nil
}
func (f *fakeClient) Diagnostics(context.Context, string) ([]lsp.Diagnostic, error) {
	return nil, nil
}
func (f *fakeClient) Close() error { return nil }
