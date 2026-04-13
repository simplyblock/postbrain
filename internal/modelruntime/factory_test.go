package modelruntime_test

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/simplyblock/postbrain/internal/config"
	"github.com/simplyblock/postbrain/internal/embedding"
	"github.com/simplyblock/postbrain/internal/modelruntime"
	"github.com/simplyblock/postbrain/internal/modelstore"
)

type fakeModelStore struct {
	models map[uuid.UUID]modelstore.ModelConfig
}

func (f *fakeModelStore) GetModelConfig(_ context.Context, modelID uuid.UUID) (*modelstore.ModelConfig, error) {
	m, ok := f.models[modelID]
	if !ok {
		return nil, nil
	}
	copy := m
	return &copy, nil
}

func TestEmbeddingFactory_EmbedderForModel_OpenAI(t *testing.T) {
	t.Parallel()

	modelID := uuid.New()
	factory := modelruntime.NewEmbeddingFactory(&config.EmbeddingConfig{
		RequestTimeout: 5 * time.Second,
		BatchSize:      16,
		Providers: map[string]config.EmbeddingProviderConfig{
			"openai-prod": {
				Backend:    "openai",
				ServiceURL: "http://localhost:8080/v1",
				APIKey:     "sk-profile",
			},
		},
	}, &fakeModelStore{models: map[uuid.UUID]modelstore.ModelConfig{
		modelID: {
			ID:             modelID,
			Provider:       "openai",
			ProviderModel:  "text-embedding-3-small",
			ProviderConfig: "openai-prod",
			Dimensions:     1536,
		},
	}})

	emb, err := factory.EmbedderForModel(context.Background(), modelID)
	if err != nil {
		t.Fatalf("EmbedderForModel: %v", err)
	}
	oa, ok := emb.(*embedding.OpenAIEmbedder)
	if !ok {
		t.Fatalf("embedder type = %T, want *embedding.OpenAIEmbedder", emb)
	}
	if oa.BaseURL() != "http://localhost:8080/v1" {
		t.Fatalf("BaseURL = %q, want model service URL", oa.BaseURL())
	}
	if oa.ModelSlug() != "text-embedding-3-small" {
		t.Fatalf("ModelSlug = %q, want provider model", oa.ModelSlug())
	}
}

func TestEmbeddingFactory_EmbedderForModel_UsesProfileOverride(t *testing.T) {
	t.Parallel()

	modelID := uuid.New()
	factory := modelruntime.NewEmbeddingFactory(&config.EmbeddingConfig{
		Providers: map[string]config.EmbeddingProviderConfig{
			"local-ollama": {
				Backend:    "ollama",
				ServiceURL: "http://localhost:11434",
			},
		},
	}, &fakeModelStore{models: map[uuid.UUID]modelstore.ModelConfig{
		modelID: {
			ID:             modelID,
			Provider:       "openai",
			ProviderConfig: "local-ollama",
			ProviderModel:  "nomic-embed-text",
			Dimensions:     768,
		},
	}})

	emb, err := factory.EmbedderForModel(context.Background(), modelID)
	if err != nil {
		t.Fatalf("EmbedderForModel: %v", err)
	}
	ool, ok := emb.(*embedding.OllamaEmbedder)
	if !ok {
		t.Fatalf("embedder type = %T, want *embedding.OllamaEmbedder", emb)
	}
	if ool.ServiceURL() != "http://localhost:11434" {
		t.Fatalf("ServiceURL = %q, want profile service URL", ool.ServiceURL())
	}
}

func TestEmbeddingFactory_EmbedderForModel_Ollama(t *testing.T) {
	t.Parallel()

	modelID := uuid.New()
	factory := modelruntime.NewEmbeddingFactory(&config.EmbeddingConfig{}, &fakeModelStore{models: map[uuid.UUID]modelstore.ModelConfig{
		modelID: {
			ID:            modelID,
			Provider:      "ollama",
			ProviderModel: "nomic-embed-text",
			ServiceURL:    "http://localhost:11434",
			Dimensions:    768,
		},
	}})

	emb, err := factory.EmbedderForModel(context.Background(), modelID)
	if err != nil {
		t.Fatalf("EmbedderForModel: %v", err)
	}
	ool, ok := emb.(*embedding.OllamaEmbedder)
	if !ok {
		t.Fatalf("embedder type = %T, want *embedding.OllamaEmbedder", emb)
	}
	if ool.ServiceURL() != "http://localhost:11434" {
		t.Fatalf("ServiceURL = %q, want model service URL", ool.ServiceURL())
	}
	if ool.ModelSlug() != "nomic-embed-text" {
		t.Fatalf("ModelSlug = %q, want provider model", ool.ModelSlug())
	}
}

func TestSummaryFactory_SummarizerForModel_UsesProfileSummaryModel(t *testing.T) {
	t.Parallel()

	modelID := uuid.New()
	factory := modelruntime.NewSummaryFactory(&config.EmbeddingConfig{
		RequestTimeout: 5 * time.Second,
		BatchSize:      16,
		Providers: map[string]config.EmbeddingProviderConfig{
			"openai-prod": {
				Backend:      "openai",
				ServiceURL:   "https://api.openai.com/v1",
				APIKey:       "sk-profile",
				SummaryModel: "gpt-4o-mini",
			},
		},
	}, &fakeModelStore{models: map[uuid.UUID]modelstore.ModelConfig{
		modelID: {
			ID:             modelID,
			Provider:       "openai",
			ProviderModel:  "text-embedding-3-small",
			ProviderConfig: "openai-prod",
			Dimensions:     1536,
		},
	}})

	sum, err := factory.SummarizerForModel(context.Background(), modelID)
	if err != nil {
		t.Fatalf("SummarizerForModel: %v", err)
	}
	oa, ok := sum.(*embedding.OpenAISummarizer)
	if !ok {
		t.Fatalf("summarizer type = %T, want *embedding.OpenAISummarizer", sum)
	}
	if oa.BaseURL() != "https://api.openai.com/v1" {
		t.Fatalf("BaseURL = %q, want profile service URL", oa.BaseURL())
	}
	if oa.ModelSlug() != "gpt-4o-mini" {
		t.Fatalf("ModelSlug = %q, want profile summary model", oa.ModelSlug())
	}
}

func TestSummaryFactory_SummarizerForModel_NoSummaryModelReturnsNil(t *testing.T) {
	t.Parallel()

	modelID := uuid.New()
	factory := modelruntime.NewSummaryFactory(&config.EmbeddingConfig{
		Providers: map[string]config.EmbeddingProviderConfig{
			"default": {
				Backend:    "openai",
				ServiceURL: "https://api.openai.com/v1",
				APIKey:     "sk-profile",
			},
		},
	}, &fakeModelStore{models: map[uuid.UUID]modelstore.ModelConfig{
		modelID: {
			ID:             modelID,
			Provider:       "openai",
			ProviderModel:  "text-embedding-3-small",
			ProviderConfig: "default",
			Dimensions:     1536,
		},
	}})

	sum, err := factory.SummarizerForModel(context.Background(), modelID)
	if err != nil {
		t.Fatalf("expected no error when summary model not configured, got: %v", err)
	}
	if sum != nil {
		t.Fatalf("expected nil summarizer when summary model not configured, got %T", sum)
	}
}

func TestEnableModelDrivenFactory_NilPoolReturnsError(t *testing.T) {
	t.Parallel()

	svc, err := embedding.NewService(&config.EmbeddingConfig{
		Providers: map[string]config.EmbeddingProviderConfig{
			"default": {Backend: "ollama", TextModel: "nomic-embed-text"},
		},
	})
	if err != nil {
		t.Fatalf("NewService: %v", err)
	}
	err = modelruntime.EnableModelDrivenFactory(context.Background(), svc, nil, &config.EmbeddingConfig{})
	if err == nil {
		t.Fatal("expected error for nil pool, got nil")
	}
}
