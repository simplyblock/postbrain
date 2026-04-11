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

	mu              sync.Mutex
	embedderCache   map[uuid.UUID]Embedder
	summarizerCache map[uuid.UUID]Summarizer
}

// ModelSummarizerFactory creates summarizers based on model metadata.
type ModelSummarizerFactory struct {
	inner *ModelEmbedderFactory
}

// NewModelEmbedderFactory constructs a model-aware embedder factory.
func NewModelEmbedderFactory(baseCfg *config.EmbeddingConfig, store ModelConfigStore) *ModelEmbedderFactory {
	cfg := &config.EmbeddingConfig{}
	if baseCfg != nil {
		copy := *baseCfg
		cfg = &copy
	}
	return &ModelEmbedderFactory{
		baseCfg:         cfg,
		store:           store,
		embedderCache:   map[uuid.UUID]Embedder{},
		summarizerCache: map[uuid.UUID]Summarizer{},
	}
}

// NewModelSummarizerFactory constructs a model-aware summarizer factory.
func NewModelSummarizerFactory(baseCfg *config.EmbeddingConfig, store ModelConfigStore) *ModelSummarizerFactory {
	return &ModelSummarizerFactory{
		inner: NewModelEmbedderFactory(baseCfg, store),
	}
}

// EmbedderForModel resolves provider settings by model ID and returns an Embedder.
func (f *ModelEmbedderFactory) EmbedderForModel(ctx context.Context, modelID uuid.UUID) (Embedder, error) {
	if f == nil || f.store == nil {
		return nil, fmt.Errorf("embedding factory: model store is not configured")
	}

	f.mu.Lock()
	cached := f.embedderCache[modelID]
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
	f.embedderCache[modelID] = emb
	f.mu.Unlock()
	return emb, nil
}

// SummarizerForModel resolves provider settings by model ID and returns a Summarizer.
// The summarizer model is resolved from the model's provider profile SummaryModel.
func (f *ModelEmbedderFactory) SummarizerForModel(ctx context.Context, modelID uuid.UUID) (Summarizer, error) {
	if f == nil || f.store == nil {
		return nil, fmt.Errorf("embedding factory: model store is not configured")
	}

	f.mu.Lock()
	cached := f.summarizerCache[modelID]
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

	sum, err := f.newSummarizerForConfig(model)
	if err != nil {
		return nil, err
	}

	f.mu.Lock()
	f.summarizerCache[modelID] = sum
	f.mu.Unlock()
	return sum, nil
}

// SummarizerForModel resolves provider settings by model ID and returns a Summarizer.
func (f *ModelSummarizerFactory) SummarizerForModel(ctx context.Context, modelID uuid.UUID) (Summarizer, error) {
	if f == nil || f.inner == nil {
		return nil, fmt.Errorf("embedding summarizer factory: model store is not configured")
	}
	return f.inner.SummarizerForModel(ctx, modelID)
}

func (f *ModelEmbedderFactory) newEmbedderForConfig(model *ModelConfig) (Embedder, error) {
	resolvedProvider, resolvedServiceURL, resolvedAPIKey, _, providerModel, err := f.resolveProviderConfig(model)
	if err != nil {
		return nil, err
	}
	cfg := *f.baseCfg

	switch resolvedProvider {
	case "ollama":
		return NewOllamaEmbedder(&cfg, providerModel, resolvedServiceURL), nil
	case "openai":
		baseURL := resolvedServiceURL
		if baseURL == "" {
			baseURL = defaultOpenAIBaseURL
		}
		if resolvedAPIKey == "" && baseURL == defaultOpenAIBaseURL {
			return nil, fmt.Errorf("embedding factory: api_key is required for default OpenAI URL")
		}
		return NewOpenAIEmbedder(&cfg, providerModel, baseURL, resolvedAPIKey), nil
	default:
		return nil, fmt.Errorf("embedding factory: unsupported provider %q", resolvedProvider)
	}
}

func (f *ModelEmbedderFactory) newSummarizerForConfig(model *ModelConfig) (Summarizer, error) {
	resolvedProvider, resolvedServiceURL, resolvedAPIKey, summaryModel, _, err := f.resolveProviderConfig(model)
	if err != nil {
		return nil, err
	}
	if summaryModel == "" {
		return nil, fmt.Errorf("embedding factory: summary_model is required in provider profile for model %s", model.ID)
	}

	cfg := *f.baseCfg
	switch resolvedProvider {
	case "ollama":
		return NewOllamaSummarizer(&cfg, summaryModel, resolvedServiceURL), nil
	case "openai":
		baseURL := resolvedServiceURL
		if baseURL == "" {
			baseURL = defaultOpenAIBaseURL
		}
		if resolvedAPIKey == "" && baseURL == defaultOpenAIBaseURL {
			return nil, fmt.Errorf("embedding factory: api_key is required for default OpenAI URL")
		}
		return NewOpenAISummarizer(&cfg, summaryModel, baseURL, resolvedAPIKey), nil
	default:
		return nil, fmt.Errorf("embedding factory: unsupported provider %q", resolvedProvider)
	}
}

func (f *ModelEmbedderFactory) resolveProviderConfig(model *ModelConfig) (provider string, serviceURL string, apiKey string, summaryModel string, providerModel string, err error) {
	profileName := strings.TrimSpace(model.ProviderConfig)
	if profileName == "" {
		profileName = "default"
	}
	resolvedProvider := strings.TrimSpace(model.Provider)
	resolvedServiceURL := strings.TrimSpace(model.ServiceURL)
	resolvedAPIKey := ""
	resolvedSummaryModel := ""
	if profile, ok := f.baseCfg.Providers[profileName]; ok {
		if strings.TrimSpace(profile.Backend) != "" {
			resolvedProvider = strings.TrimSpace(profile.Backend)
		}
		if strings.TrimSpace(profile.ServiceURL) != "" {
			resolvedServiceURL = strings.TrimSpace(profile.ServiceURL)
		}
		if strings.TrimSpace(profile.APIKey) != "" {
			resolvedAPIKey = strings.TrimSpace(profile.APIKey)
		}
		if strings.TrimSpace(profile.SummaryModel) != "" {
			resolvedSummaryModel = strings.TrimSpace(profile.SummaryModel)
		}
	} else if profileName != "default" {
		return "", "", "", "", "", fmt.Errorf("embedding factory: unknown provider profile %q for model %s", profileName, model.ID)
	}
	provider = strings.ToLower(strings.TrimSpace(resolvedProvider))
	providerModel = strings.TrimSpace(model.ProviderModel)
	if providerModel == "" {
		return "", "", "", "", "", fmt.Errorf("embedding factory: model %s has empty provider_model", model.ID)
	}
	return provider, resolvedServiceURL, resolvedAPIKey, resolvedSummaryModel, providerModel, nil
}
