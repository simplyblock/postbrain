package codegraph

import (
	"context"
	"errors"
	"testing"
	"time"
)

func TestLSPResolverForIndex_EmptyAddress_Disabled(t *testing.T) {
	t.Parallel()

	called := false
	prev := newGoplsTCPResolverFn
	newGoplsTCPResolverFn = func(string, time.Duration, string) (LSPResolver, error) {
		called = true
		return nil, nil
	}
	t.Cleanup(func() { newGoplsTCPResolverFn = prev })

	got := lspResolverForIndex(context.Background(), IndexOptions{})
	if got != nil {
		t.Fatal("expected nil resolver when GoLSPAddr is empty")
	}
	if called {
		t.Fatal("did not expect resolver constructor call")
	}
}

func TestLSPResolverForIndex_Enabled_ReturnsResolver(t *testing.T) {
	t.Parallel()

	want := &fakeLSPResolver{lang: ".go"}
	prev := newGoplsTCPResolverFn
	newGoplsTCPResolverFn = func(addr string, timeout time.Duration, rootURI string) (LSPResolver, error) {
		if addr != "127.0.0.1:37373" {
			t.Fatalf("addr = %q, want %q", addr, "127.0.0.1:37373")
		}
		if timeout != 3*time.Second {
			t.Fatalf("timeout = %v, want %v", timeout, 3*time.Second)
		}
		if rootURI != "file:///tmp/repo" {
			t.Fatalf("rootURI = %q, want %q", rootURI, "file:///tmp/repo")
		}
		return want, nil
	}
	t.Cleanup(func() { newGoplsTCPResolverFn = prev })

	got := lspResolverForIndex(context.Background(), IndexOptions{
		GoLSPAddr:    "127.0.0.1:37373",
		GoLSPRootURI: "file:///tmp/repo",
		GoLSPTimeout: 3 * time.Second,
	})
	if got != want {
		t.Fatal("expected returned resolver from constructor")
	}
}

func TestLSPResolverForIndex_ConstructorError_FallsBackToNil(t *testing.T) {
	t.Parallel()

	prev := newGoplsTCPResolverFn
	newGoplsTCPResolverFn = func(string, time.Duration, string) (LSPResolver, error) {
		return nil, errors.New("dial failed")
	}
	t.Cleanup(func() { newGoplsTCPResolverFn = prev })

	got := lspResolverForIndex(context.Background(), IndexOptions{GoLSPAddr: "127.0.0.1:37373"})
	if got != nil {
		t.Fatal("expected nil resolver on constructor error")
	}
}
