package embedding

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/simplyblock/postbrain/internal/config"
)

type fakeModelStore struct {
	models map[uuid.UUID]ModelConfig
}

type fakeResolver struct {
	embedder Embedder
}

func (f fakeResolver) EmbedderForModel(ctx context.Context, modelID uuid.UUID) (Embedder, error) {
	_ = ctx
	_ = modelID
	return f.embedder, nil
}

func (f *fakeModelStore) GetModelConfig(ctx context.Context, modelID uuid.UUID) (*ModelConfig, error) {
	_ = ctx
	m, ok := f.models[modelID]
	if !ok {
		return nil, nil
	}
	copy := m
	return &copy, nil
}

func TestModelEmbedderFactory_EmbedderForModel_OpenAI(t *testing.T) {
	t.Parallel()

	modelID := uuid.New()
	factory := NewModelEmbedderFactory(&config.EmbeddingConfig{
		RequestTimeout: 5 * time.Second,
		BatchSize:      16,
		Providers: map[string]config.EmbeddingProviderConfig{
			"openai-prod": {
				Backend:    "openai",
				ServiceURL: "http://localhost:8080/v1",
				APIKey:     "sk-profile",
			},
		},
	}, &fakeModelStore{models: map[uuid.UUID]ModelConfig{
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
	oa, ok := emb.(*OpenAIEmbedder)
	if !ok {
		t.Fatalf("embedder type = %T, want *OpenAIEmbedder", emb)
	}
	if oa.baseURL != "http://localhost:8080/v1" {
		t.Fatalf("baseURL = %q, want model service URL", oa.baseURL)
	}
	if oa.modelSlug != "text-embedding-3-small" {
		t.Fatalf("modelSlug = %q, want provider model", oa.modelSlug)
	}
}

func TestModelEmbedderFactory_EmbedderForModel_UsesProfileOverride(t *testing.T) {
	t.Parallel()

	modelID := uuid.New()
	factory := NewModelEmbedderFactory(&config.EmbeddingConfig{
		Providers: map[string]config.EmbeddingProviderConfig{
			"local-ollama": {
				Backend:    "ollama",
				ServiceURL: "http://localhost:11434",
			},
		},
	}, &fakeModelStore{models: map[uuid.UUID]ModelConfig{
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
	ool, ok := emb.(*OllamaEmbedder)
	if !ok {
		t.Fatalf("embedder type = %T, want *OllamaEmbedder", emb)
	}
	if got := serviceURLOrDefault(ool.serviceURL, defaultOllamaServiceURL); got != "http://localhost:11434" {
		t.Fatalf("service URL = %q, want profile service URL", got)
	}
}

func TestModelEmbedderFactory_EmbedderForModel_Ollama(t *testing.T) {
	t.Parallel()

	modelID := uuid.New()
	factory := NewModelEmbedderFactory(&config.EmbeddingConfig{}, &fakeModelStore{models: map[uuid.UUID]ModelConfig{
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
	ool, ok := emb.(*OllamaEmbedder)
	if !ok {
		t.Fatalf("embedder type = %T, want *OllamaEmbedder", emb)
	}
	if got := serviceURLOrDefault(ool.serviceURL, defaultOllamaServiceURL); got != "http://localhost:11434" {
		t.Fatalf("service URL = %q, want model service URL", got)
	}
	if ool.modelSlug != "nomic-embed-text" {
		t.Fatalf("modelSlug = %q, want provider model", ool.modelSlug)
	}
}

func TestEmbeddingService_EmbedTextResult_UsesActiveModelFactory(t *testing.T) {
	t.Parallel()

	modelID := uuid.New()
	svc := NewServiceFromEmbedders(&mockEmbedder{vec: []float32{9, 9}}, nil)
	svc.SetModelFactory(
		fakeResolver{embedder: &mockEmbedder{vec: []float32{1, 2, 3}}},
		&modelID,
		nil,
	)

	res, err := svc.EmbedTextResult(context.Background(), "hello")
	if err != nil {
		t.Fatalf("EmbedTextResult: %v", err)
	}
	if res == nil {
		t.Fatal("EmbedTextResult returned nil result")
	}
	if res.ModelID != modelID {
		t.Fatalf("ModelID = %s, want %s", res.ModelID, modelID)
	}
	if len(res.Embedding) != 3 || res.Embedding[0] != 1 {
		t.Fatalf("Embedding = %#v, want [1 2 3]", res.Embedding)
	}
}

func TestEmbeddingService_EmbedCodeResult_UsesActiveModelFactory(t *testing.T) {
	t.Parallel()

	modelID := uuid.New()
	svc := NewServiceFromEmbedders(&mockEmbedder{vec: []float32{9, 9}}, &mockEmbedder{vec: []float32{8, 8}})
	svc.SetModelFactory(
		fakeResolver{embedder: &mockEmbedder{vec: []float32{4, 5}}},
		nil,
		&modelID,
	)

	res, err := svc.EmbedCodeResult(context.Background(), "hello")
	if err != nil {
		t.Fatalf("EmbedCodeResult: %v", err)
	}
	if res == nil {
		t.Fatal("EmbedCodeResult returned nil result")
	}
	if res.ModelID != modelID {
		t.Fatalf("ModelID = %s, want %s", res.ModelID, modelID)
	}
	if len(res.Embedding) != 2 || res.Embedding[0] != 4 {
		t.Fatalf("Embedding = %#v, want [4 5]", res.Embedding)
	}
}
