package codegraph

import (
	"context"
	"testing"

	"github.com/google/uuid"
)

type fakeLSPResolver struct {
	lang      string
	canonical string
	calls     int
}

func (f *fakeLSPResolver) Language() string { return f.lang }

func (f *fakeLSPResolver) Resolve(context.Context, string, string) (string, error) {
	f.calls++
	return f.canonical, nil
}

func (f *fakeLSPResolver) Close() error { return nil }

func TestResolver_Resolve_CallsLSPWhenLocalAndImportMiss(t *testing.T) {
	t.Parallel()

	lsp := &fakeLSPResolver{lang: ".go", canonical: "pkg.Target"}
	r := NewResolver(nil, uuid.New(), lsp)

	_, ok := r.Resolve(context.Background(), "some/file.go", "Target", nil)
	if ok {
		t.Fatal("expected unresolved result without DB-backed canonical lookup")
	}
	if lsp.calls != 1 {
		t.Fatalf("lsp calls = %d, want 1", lsp.calls)
	}
}

func TestResolver_Resolve_DoesNotCallLSPForDifferentLanguage(t *testing.T) {
	t.Parallel()

	lsp := &fakeLSPResolver{lang: ".go", canonical: "pkg.Target"}
	r := NewResolver(nil, uuid.New(), lsp)

	_, ok := r.Resolve(context.Background(), "some/file.py", "Target", nil)
	if ok {
		t.Fatal("expected unresolved result")
	}
	if lsp.calls != 0 {
		t.Fatalf("lsp calls = %d, want 0", lsp.calls)
	}
}

func TestResolver_Resolve_LocalHitSkipsLSP(t *testing.T) {
	t.Parallel()

	lsp := &fakeLSPResolver{lang: ".go", canonical: "pkg.Target"}
	r := NewResolver(nil, uuid.New(), lsp)

	want := uuid.New()
	local := map[string]uuid.UUID{"Target": want}

	got, ok := r.Resolve(context.Background(), "some/file.go", "Target", local)
	if !ok {
		t.Fatal("expected local hit")
	}
	if got != want {
		t.Fatalf("got %v, want %v", got, want)
	}
	if lsp.calls != 0 {
		t.Fatalf("lsp calls = %d, want 0", lsp.calls)
	}
}
