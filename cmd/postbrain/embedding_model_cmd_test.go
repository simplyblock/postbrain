package main

import (
	"bytes"
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/simplyblock/postbrain/internal/config"
)

func TestEmbeddingModelRegisterCommand_RequiresSlug(t *testing.T) {
	root := newRootCmd()
	root.SetArgs([]string{"embedding-model", "register", "--dimensions", "1536", "--content-type", "text"})

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
	root.SetArgs([]string{"embedding-model", "register", "--slug", "text-1", "--dimensions", "1536", "--content-type", "text"})

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

func TestEmbeddingModelRegisterCommand_BackendError(t *testing.T) {
	old := registerEmbeddingModelCmdFn
	registerEmbeddingModelCmdFn = func(context.Context, embeddingModelRegisterOptions) (string, error) {
		return "", errors.New("boom")
	}
	t.Cleanup(func() { registerEmbeddingModelCmdFn = old })

	root := newRootCmd()
	root.SetArgs([]string{"embedding-model", "register", "--slug", "text-1", "--dimensions", "1536", "--content-type", "text"})

	if err := root.Execute(); err == nil {
		t.Fatal("expected error, got nil")
	}
}

func TestResolveProviderRegistrationFields_OpenAIDefaultServiceURL(t *testing.T) {
	t.Parallel()
	got, err := resolveProviderRegistrationFields(embeddingModelRegisterOptions{
		ProviderConfig: "openai-prod",
		ContentType:    "text",
	}, &config.EmbeddingConfig{
		Providers: map[string]config.EmbeddingProviderConfig{
			"openai-prod": {
				Backend:   "openai",
				TextModel: "text-embedding-3-small",
			},
		},
	}, "embedding")
	if err != nil {
		t.Fatalf("resolveProviderRegistrationFields: %v", err)
	}
	if got.ServiceURL != "https://api.openai.com/v1" {
		t.Fatalf("service_url = %q, want https://api.openai.com/v1", got.ServiceURL)
	}
}

func TestResolveProviderRegistrationFields_MissingServiceURLFails(t *testing.T) {
	t.Parallel()
	_, err := resolveProviderRegistrationFields(embeddingModelRegisterOptions{
		ProviderConfig: "default",
		ContentType:    "text",
	}, &config.EmbeddingConfig{
		Providers: map[string]config.EmbeddingProviderConfig{
			"default": {
				Backend:   "ollama",
				TextModel: "nomic-embed-text",
			},
		},
	}, "embedding")
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !strings.Contains(err.Error(), "service_url is required") {
		t.Fatalf("unexpected error: %v", err)
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

func TestSummaryModelRegisterCommand_Success(t *testing.T) {
	old := registerSummaryModelCmdFn
	registerSummaryModelCmdFn = func(ctx context.Context, opts embeddingModelRegisterOptions) (string, error) {
		if opts.Slug != "summary-1" {
			t.Fatalf("slug = %q, want summary-1", opts.Slug)
		}
		if opts.ProviderConfig != "default" {
			t.Fatalf("provider-config = %q, want default", opts.ProviderConfig)
		}
		if opts.ContentType != "text" {
			t.Fatalf("content-type = %q, want text", opts.ContentType)
		}
		if opts.Dimensions != 1 {
			t.Fatalf("dimensions = %d, want default 1", opts.Dimensions)
		}
		return "registered model summary-1", nil
	}
	t.Cleanup(func() { registerSummaryModelCmdFn = old })

	root := newRootCmd()
	root.SetArgs([]string{"summary-model", "register", "--slug", "summary-1"})

	var out bytes.Buffer
	root.SetOut(&out)
	root.SetErr(&out)

	if err := root.Execute(); err != nil {
		t.Fatalf("execute summary register command: %v", err)
	}
	if got := strings.TrimSpace(out.String()); got != "registered model summary-1" {
		t.Fatalf("output = %q, want %q", got, "registered model summary-1")
	}
}

func TestSummaryModelActivateCommand_Success(t *testing.T) {
	old := activateSummaryModelCmdFn
	activateSummaryModelCmdFn = func(ctx context.Context, opts embeddingModelActivateOptions) (string, error) {
		if opts.Slug != "summary-1" || opts.ContentType != "text" {
			t.Fatalf("unexpected opts: %+v", opts)
		}
		return "activated model summary-1 for text", nil
	}
	t.Cleanup(func() { activateSummaryModelCmdFn = old })

	root := newRootCmd()
	root.SetArgs([]string{"summary-model", "activate", "--slug", "summary-1"})

	var out bytes.Buffer
	root.SetOut(&out)
	root.SetErr(&out)

	if err := root.Execute(); err != nil {
		t.Fatalf("execute summary activate command: %v", err)
	}
	if got := strings.TrimSpace(out.String()); got != "activated model summary-1 for text" {
		t.Fatalf("output = %q, want %q", got, "activated model summary-1 for text")
	}
}

func TestSummaryModelListCommand_Success(t *testing.T) {
	old := listSummaryModelCmdFn
	listSummaryModelCmdFn = func(context.Context, embeddingModelListOptions) (string, error) {
		return "slug\tprovider\nsummary-1\topenai", nil
	}
	t.Cleanup(func() { listSummaryModelCmdFn = old })

	root := newRootCmd()
	root.SetArgs([]string{"summary-model", "list"})

	var out bytes.Buffer
	root.SetOut(&out)
	root.SetErr(&out)

	if err := root.Execute(); err != nil {
		t.Fatalf("execute summary list command: %v", err)
	}
	if got := strings.TrimSpace(out.String()); got != "slug\tprovider\nsummary-1\topenai" {
		t.Fatalf("output = %q, want list payload", got)
	}
}

func TestResolveProviderRegistrationFields_GenerationUsesSummaryModel(t *testing.T) {
	t.Parallel()
	got, err := resolveProviderRegistrationFields(embeddingModelRegisterOptions{
		ProviderConfig: "openai-prod",
		ContentType:    "text",
	}, &config.EmbeddingConfig{
		Providers: map[string]config.EmbeddingProviderConfig{
			"openai-prod": {
				Backend:      "openai",
				SummaryModel: "gpt-4o-mini",
			},
		},
	}, "generation")
	if err != nil {
		t.Fatalf("resolveProviderRegistrationFields: %v", err)
	}
	if got.ProviderModel != "gpt-4o-mini" {
		t.Fatalf("provider_model = %q, want gpt-4o-mini", got.ProviderModel)
	}
}

func TestResolveProviderRegistrationFields_GenerationMissingSummaryModelFails(t *testing.T) {
	t.Parallel()
	_, err := resolveProviderRegistrationFields(embeddingModelRegisterOptions{
		ProviderConfig: "default",
		ContentType:    "text",
	}, &config.EmbeddingConfig{
		Providers: map[string]config.EmbeddingProviderConfig{
			"default": {
				Backend: "openai",
			},
		},
	}, "generation")
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !strings.Contains(err.Error(), "summary_model is required") {
		t.Fatalf("unexpected error: %v", err)
	}
}
