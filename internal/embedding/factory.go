package embedding

import (
	"context"
	"fmt"
	"strings"
	"sync"

	"github.com/google/uuid"
	"github.com/simplyblock/postbrain/internal/config"
)

// ModelConfig contains the provider/runtime settings for one embedding model.
type ModelConfig struct {
	ID             uuid.UUID
	Provider       string
	ProviderConfig string
	ServiceURL     string
	ProviderModel  string
	Dimensions     int
}

// ModelConfigStore loads model configuration for a model ID.
type ModelConfigStore interface {
	GetModelConfig(ctx context.Context, modelID uuid.UUID) (*ModelConfig, error)
}

// ModelEmbedderFactory creates embedders based on model metadata.
type ModelEmbedderFactory struct {
	baseCfg *config.EmbeddingConfig
	store   ModelConfigStore

	mu    sync.Mutex
	cache map[uuid.UUID]Embedder
}

// NewModelEmbedderFactory constructs a model-aware embedder factory.
func NewModelEmbedderFactory(baseCfg *config.EmbeddingConfig, store ModelConfigStore) *ModelEmbedderFactory {
	cfg := &config.EmbeddingConfig{}
	if baseCfg != nil {
		copy := *baseCfg
		cfg = &copy
	}
	return &ModelEmbedderFactory{
		baseCfg: cfg,
		store:   store,
		cache:   map[uuid.UUID]Embedder{},
	}
}

// EmbedderForModel resolves provider settings by model ID and returns an Embedder.
func (f *ModelEmbedderFactory) EmbedderForModel(ctx context.Context, modelID uuid.UUID) (Embedder, error) {
	if f == nil || f.store == nil {
		return nil, fmt.Errorf("embedding factory: model store is not configured")
	}

	f.mu.Lock()
	cached := f.cache[modelID]
	f.mu.Unlock()
	if cached != nil {
		return cached, nil
	}

	model, err := f.store.GetModelConfig(ctx, modelID)
	if err != nil {
		return nil, fmt.Errorf("embedding factory: load model %s: %w", modelID, err)
	}
	if model == nil {
		return nil, fmt.Errorf("embedding factory: model %s not found", modelID)
	}

	emb, err := f.newEmbedderForConfig(model)
	if err != nil {
		return nil, err
	}

	f.mu.Lock()
	f.cache[modelID] = emb
	f.mu.Unlock()
	return emb, nil
}

func (f *ModelEmbedderFactory) newEmbedderForConfig(model *ModelConfig) (Embedder, error) {
	profileName := strings.TrimSpace(model.ProviderConfig)
	if profileName == "" {
		profileName = "default"
	}
	resolvedProvider := strings.TrimSpace(model.Provider)
	resolvedServiceURL := strings.TrimSpace(model.ServiceURL)
	resolvedAPIKey := strings.TrimSpace(f.baseCfg.OpenAIAPIKey)
	if profile, ok := f.baseCfg.Providers[profileName]; ok {
		if strings.TrimSpace(profile.Backend) != "" {
			resolvedProvider = strings.TrimSpace(profile.Backend)
		}
		if strings.TrimSpace(profile.ServiceURL) != "" {
			resolvedServiceURL = strings.TrimSpace(profile.ServiceURL)
		}
		if strings.TrimSpace(profile.OpenAIAPIKey) != "" {
			resolvedAPIKey = strings.TrimSpace(profile.OpenAIAPIKey)
		}
	} else if profileName != "default" {
		return nil, fmt.Errorf("embedding factory: unknown provider profile %q for model %s", profileName, model.ID)
	}
	provider := strings.ToLower(strings.TrimSpace(resolvedProvider))
	providerModel := strings.TrimSpace(model.ProviderModel)
	if providerModel == "" {
		return nil, fmt.Errorf("embedding factory: model %s has empty provider_model", model.ID)
	}

	cfg := *f.baseCfg
	cfg.ServiceURL = resolvedServiceURL
	cfg.OpenAIAPIKey = resolvedAPIKey

	switch provider {
	case "ollama":
		if cfg.ServiceURL == "" {
			cfg.ServiceURL = defaultOllamaServiceURL
		}
		return NewOllamaEmbedder(&cfg, providerModel), nil
	case "openai":
		baseURL := cfg.ServiceURL
		if baseURL == "" {
			baseURL = defaultOpenAIBaseURL
		}
		if cfg.OpenAIAPIKey == "" && baseURL == defaultOpenAIBaseURL {
			return nil, fmt.Errorf("embedding factory: openai_api_key is required for default OpenAI URL")
		}
		return NewOpenAIEmbedder(&cfg, providerModel, baseURL), nil
	default:
		return nil, fmt.Errorf("embedding factory: unsupported provider %q", provider)
	}
}
