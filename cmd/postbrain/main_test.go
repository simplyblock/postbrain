package main

import (
	"bytes"
	"slices"
	"strings"
	"testing"

	"github.com/simplyblock/postbrain/internal/config"
)

func TestRootVersionCommand_PrintsBuildVersion(t *testing.T) {
	t.Parallel()

	old := buildVersion
	oldRef := buildGitRef
	oldTime := buildTimestamp
	buildVersion = "1.2.3-test"
	buildGitRef = "abc1234"
	buildTimestamp = "2026-04-03T14:30:00Z"
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
	want := "version=1.2.3-test git=abc1234 built=2026-04-03T14:30:00Z"
	if got != want {
		t.Fatalf("version output = %q, want %q", got, want)
	}
}

func TestEnabledJobNames_ReturnsEnabledOnly(t *testing.T) {
	t.Parallel()
	got := enabledJobNames(config.JobsConfig{
		ConsolidationEnabled:     true,
		ContradictionEnabled:     false,
		ReembedEnabled:           true,
		AgeCheckEnabled:          true,
		BackfillSummariesEnabled: false,
		ChunkBackfillEnabled:     true,
	})
	want := []string{"consolidation", "reembed", "age_check", "backfill_chunks"}
	if !slices.Equal(got, want) {
		t.Fatalf("enabledJobNames() = %#v, want %#v", got, want)
	}
}

func TestEmbeddingProviderInfos_SortsAndMasksSecrets(t *testing.T) {
	t.Parallel()
	got := embeddingProviderInfos(config.EmbeddingConfig{
		Providers: map[string]config.EmbeddingProviderConfig{
			"z-local": {
				Backend:    "ollama",
				ServiceURL: "http://localhost:11434",
				TextModel:  "nomic-embed-text",
			},
			"a-openai": {
				Backend:      "openai",
				ServiceURL:   "",
				APIKey:       "super-secret",
				TextModel:    "text-embedding-3-small",
				CodeModel:    "text-embedding-3-large",
				SummaryModel: "gpt-4.1-mini",
			},
		},
	})
	if len(got) != 2 {
		t.Fatalf("len(infos) = %d, want 2", len(got))
	}
	if got[0].Name != "a-openai" || got[1].Name != "z-local" {
		t.Fatalf("provider ordering = %#v, want alphabetical by name", got)
	}
	if !got[0].HasAPIKey {
		t.Fatal("expected HasAPIKey=true for a-openai")
	}
	if got[1].HasAPIKey {
		t.Fatal("expected HasAPIKey=false for z-local")
	}
}

func TestEnabledOAuthProviderNames_ReturnsSortedEnabledProviders(t *testing.T) {
	t.Parallel()
	got := enabledOAuthProviderNames(config.OAuthConfig{
		Providers: map[string]config.ProviderConfig{
			"github": {Enabled: true},
			"google": {Enabled: false},
			"gitlab": {Enabled: true},
		},
	})
	want := []string{"github", "gitlab"}
	if !slices.Equal(got, want) {
		t.Fatalf("enabledOAuthProviderNames() = %#v, want %#v", got, want)
	}
}
