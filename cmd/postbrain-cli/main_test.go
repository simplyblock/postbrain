package main

import (
	"bytes"
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/google/uuid"
)

// ── parseSkillID ──────────────────────────────────────────────────────────────

func TestParseSkillID_ValidUUID_ReturnsID(t *testing.T) {
	t.Parallel()
	want := uuid.New()
	got, err := parseSkillID(want.String())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != want {
		t.Errorf("got %v, want %v", got, want)
	}
}

func TestParseSkillID_InvalidUUID_ReturnsError(t *testing.T) {
	t.Parallel()
	_, err := parseSkillID("not-a-uuid")
	if err == nil {
		t.Fatal("expected error for invalid UUID, got nil")
	}
}

func TestParseSkillID_EmptyString_ReturnsError(t *testing.T) {
	t.Parallel()
	_, err := parseSkillID("")
	if err == nil {
		t.Fatal("expected error for empty string, got nil")
	}
}

func TestRootVersionCommand_PrintsBuildVersion(t *testing.T) {
	t.Parallel()

	old := buildVersion
	oldRef := buildGitRef
	oldTime := buildTimestamp
	buildVersion = "9.8.7-test"
	buildGitRef = "def5678"
	buildTimestamp = "2026-04-03T14:31:00Z"
	t.Cleanup(func() { buildVersion = old })
	t.Cleanup(func() { buildGitRef = oldRef })
	t.Cleanup(func() { buildTimestamp = oldTime })

	root := newRootCmd()
	root.SetArgs([]string{"version"})

	var out bytes.Buffer
	root.SetOut(&out)
	root.SetErr(&out)

	if err := root.Execute(); err != nil {
		t.Fatalf("execute version command: %v", err)
	}

	got := strings.TrimSpace(out.String())
	want := "version=9.8.7-test git=def5678 built=2026-04-03T14:31:00Z"
	if got != want {
		t.Fatalf("version output = %q, want %q", got, want)
	}
}

func TestEmbeddingModelRegisterCommand_RequiresSlug(t *testing.T) {
	root := newRootCmd()
	root.SetArgs([]string{"embedding-model", "register", "--provider", "openai", "--service-url", "http://localhost:11434/v1", "--provider-model", "text-embedding-3-large", "--dimensions", "1536", "--content-type", "text"})

	var out bytes.Buffer
	root.SetOut(&out)
	root.SetErr(&out)

	err := root.Execute()
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !strings.Contains(err.Error(), "required flag(s) \"slug\" not set") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestEmbeddingModelRegisterCommand_Success(t *testing.T) {
	old := registerEmbeddingModelCmdFn
	registerEmbeddingModelCmdFn = func(ctx context.Context, opts embeddingModelRegisterOptions) (string, error) {
		if opts.Slug != "text-1" {
			t.Fatalf("slug = %q, want text-1", opts.Slug)
		}
		if opts.ProviderConfig != "default" {
			t.Fatalf("provider-config = %q, want default", opts.ProviderConfig)
		}
		return "registered model text-1", nil
	}
	t.Cleanup(func() { registerEmbeddingModelCmdFn = old })

	root := newRootCmd()
	root.SetArgs([]string{"embedding-model", "register", "--slug", "text-1", "--provider", "openai", "--service-url", "http://localhost:11434/v1", "--provider-model", "text-embedding-3-large", "--dimensions", "1536", "--content-type", "text"})

	var out bytes.Buffer
	root.SetOut(&out)
	root.SetErr(&out)

	if err := root.Execute(); err != nil {
		t.Fatalf("execute register command: %v", err)
	}
	if got := strings.TrimSpace(out.String()); got != "registered model text-1" {
		t.Fatalf("output = %q, want %q", got, "registered model text-1")
	}
}

func TestEmbeddingModelRegisterCommand_ProviderConfigOverride(t *testing.T) {
	old := registerEmbeddingModelCmdFn
	registerEmbeddingModelCmdFn = func(ctx context.Context, opts embeddingModelRegisterOptions) (string, error) {
		if opts.ProviderConfig != "openai-prod" {
			t.Fatalf("provider-config = %q, want openai-prod", opts.ProviderConfig)
		}
		return "registered model text-1", nil
	}
	t.Cleanup(func() { registerEmbeddingModelCmdFn = old })

	root := newRootCmd()
	root.SetArgs([]string{"embedding-model", "register", "--slug", "text-1", "--provider", "openai", "--service-url", "http://localhost:11434/v1", "--provider-model", "text-embedding-3-large", "--provider-config", "openai-prod", "--dimensions", "1536", "--content-type", "text"})

	if err := root.Execute(); err != nil {
		t.Fatalf("execute register command: %v", err)
	}
}

func TestEmbeddingModelRegisterCommand_BackendError(t *testing.T) {
	old := registerEmbeddingModelCmdFn
	registerEmbeddingModelCmdFn = func(context.Context, embeddingModelRegisterOptions) (string, error) {
		return "", errors.New("boom")
	}
	t.Cleanup(func() { registerEmbeddingModelCmdFn = old })

	root := newRootCmd()
	root.SetArgs([]string{"embedding-model", "register", "--slug", "text-1", "--provider", "openai", "--service-url", "http://localhost:11434/v1", "--provider-model", "text-embedding-3-large", "--dimensions", "1536", "--content-type", "text"})

	if err := root.Execute(); err == nil {
		t.Fatal("expected error, got nil")
	}
}

func TestEmbeddingModelActivateCommand_RequiresSlug(t *testing.T) {
	root := newRootCmd()
	root.SetArgs([]string{"embedding-model", "activate", "--content-type", "text"})

	err := root.Execute()
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !strings.Contains(err.Error(), "required flag(s) \"slug\" not set") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestEmbeddingModelActivateCommand_Success(t *testing.T) {
	old := activateEmbeddingModelCmdFn
	activateEmbeddingModelCmdFn = func(ctx context.Context, opts embeddingModelActivateOptions) (string, error) {
		if opts.Slug != "text-1" || opts.ContentType != "text" {
			t.Fatalf("unexpected opts: %+v", opts)
		}
		return "activated model text-1 for text", nil
	}
	t.Cleanup(func() { activateEmbeddingModelCmdFn = old })

	root := newRootCmd()
	root.SetArgs([]string{"embedding-model", "activate", "--slug", "text-1", "--content-type", "text"})

	var out bytes.Buffer
	root.SetOut(&out)
	root.SetErr(&out)

	if err := root.Execute(); err != nil {
		t.Fatalf("execute activate command: %v", err)
	}
	if got := strings.TrimSpace(out.String()); got != "activated model text-1 for text" {
		t.Fatalf("output = %q, want %q", got, "activated model text-1 for text")
	}
}

func TestEmbeddingModelListCommand_Success(t *testing.T) {
	old := listEmbeddingModelCmdFn
	listEmbeddingModelCmdFn = func(context.Context, embeddingModelListOptions) (string, error) {
		return "slug\tprovider\ntext-1\topenai", nil
	}
	t.Cleanup(func() { listEmbeddingModelCmdFn = old })

	root := newRootCmd()
	root.SetArgs([]string{"embedding-model", "list"})

	var out bytes.Buffer
	root.SetOut(&out)
	root.SetErr(&out)

	if err := root.Execute(); err != nil {
		t.Fatalf("execute list command: %v", err)
	}
	if got := strings.TrimSpace(out.String()); got != "slug\tprovider\ntext-1\topenai" {
		t.Fatalf("output = %q, want list payload", got)
	}
}
