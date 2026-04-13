package lsp

import (
	"errors"
	"testing"
	"time"
)

func TestNewClientForExt_TypeScript_DefaultsToTypeScriptLanguageServer(t *testing.T) {
	t.Parallel()

	origTLS := newTypeScriptLanguageServerClient
	origTSGo := newTSGoClient
	newTypeScriptLanguageServerClient = func(rootDir string, timeout time.Duration) (Client, error) {
		return &PyrightClient{}, nil
	}
	newTSGoClient = func(rootDir string, timeout time.Duration) (Client, error) {
		t.Fatal("tsgo constructor must not be called without flag")
		return nil, nil
	}
	t.Cleanup(func() {
		newTypeScriptLanguageServerClient = origTLS
		newTSGoClient = origTSGo
	})

	got, err := NewClientForExt(".ts", "/tmp/repo", 3*time.Second, ClientOptions{})
	if err != nil {
		t.Fatalf("NewClientForExt: %v", err)
	}
	if got == nil {
		t.Fatal("expected client")
	}
}

func TestNewClientForExt_TypeScript_UsesTSGoWhenFlagEnabled(t *testing.T) {
	t.Parallel()

	wantErr := errors.New("boom")
	origTLS := newTypeScriptLanguageServerClient
	origTSGo := newTSGoClient
	newTypeScriptLanguageServerClient = func(rootDir string, timeout time.Duration) (Client, error) {
		t.Fatal("typescript-language-server constructor must not be called with tsgo flag")
		return nil, nil
	}
	newTSGoClient = func(rootDir string, timeout time.Duration) (Client, error) {
		return nil, wantErr
	}
	t.Cleanup(func() {
		newTypeScriptLanguageServerClient = origTLS
		newTSGoClient = origTSGo
	})

	got, err := NewClientForExt(".ts", "/tmp/repo", 3*time.Second, ClientOptions{UseTSGo: true})
	if !errors.Is(err, wantErr) {
		t.Fatalf("err = %v, want %v", err, wantErr)
	}
	if got != nil {
		t.Fatal("expected nil client on constructor error")
	}
}

func TestNewClientForExt_CCpp_UsesClangd(t *testing.T) {
	t.Parallel()

	origClangd := newClangdClient
	newClangdClient = func(rootDir string, timeout time.Duration) (Client, error) {
		return &PyrightClient{}, nil
	}
	t.Cleanup(func() {
		newClangdClient = origClangd
	})

	got, err := NewClientForExt(".cpp", "/tmp/repo", 3*time.Second, ClientOptions{})
	if err != nil {
		t.Fatalf("NewClientForExt: %v", err)
	}
	if got == nil {
		t.Fatal("expected client")
	}
}
