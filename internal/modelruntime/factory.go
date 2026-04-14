// Package modelruntime provides model-aware factory construction for embedders
// and summarizers. It bridges model registry lookup (modelstore) with provider
// implementations (providers) and is the correct injection point for
// model-driven factory wiring.
package modelruntime

import (
	"context"
	"fmt"
	"strings"
	"sync"

	"github.com/google/uuid"

	"github.com/simplyblock/postbrain/internal/config"
	"github.com/simplyblock/postbrain/internal/modelstore"
	"github.com/simplyblock/postbrain/internal/providers"
)

const defaultOpenAIBaseURL = "https://api.openai.com/v1"

// ModelConfigStore loads model configuration for a model ID.
// Both modelstore.EmbeddingModelStore and modelstore.GenerationModelStore satisfy this.
type ModelConfigStore interface {
	GetModelConfig(ctx context.Context, modelID uuid.UUID) (*modelstore.ModelConfig, error)
}

// EmbeddingFactory creates Embedder instances by model ID.
// Implements providers.EmbedderResolver.
type EmbeddingFactory struct {
	baseCfg *config.EmbeddingConfig
	store   ModelConfigStore

	mu    sync.Mutex
	cache map[uuid.UUID]providers.Embedder
}

// NewEmbeddingFactory constructs a model-aware embedder factory.
func NewEmbeddingFactory(baseCfg *config.EmbeddingConfig, store ModelConfigStore) *EmbeddingFactory {
	cfg := &config.EmbeddingConfig{}
	if baseCfg != nil {
		copy := *baseCfg
		cfg = &copy
	}
	return &EmbeddingFactory{
		baseCfg: cfg,
		store:   store,
		cache:   make(map[uuid.UUID]providers.Embedder),
	}
}

// EmbedderForModel resolves provider settings by model ID and returns an Embedder.
func (f *EmbeddingFactory) EmbedderForModel(ctx context.Context, modelID uuid.UUID) (providers.Embedder, error) {
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
	emb, err := newEmbedderForConfig(f.baseCfg, model)
	if err != nil {
		return nil, err
	}
	f.mu.Lock()
	f.cache[modelID] = emb
	f.mu.Unlock()
	return emb, nil
}

// SummaryFactory creates Summarizer instances by model ID.
// Implements providers.SummarizerResolver.
type SummaryFactory struct {
	baseCfg *config.EmbeddingConfig
	store   ModelConfigStore

	mu    sync.Mutex
	cache map[uuid.UUID]providers.Summarizer
}

// NewSummaryFactory constructs a model-aware summarizer factory.
func NewSummaryFactory(baseCfg *config.EmbeddingConfig, store ModelConfigStore) *SummaryFactory {
	cfg := &config.EmbeddingConfig{}
	if baseCfg != nil {
		copy := *baseCfg
		cfg = &copy
	}
	return &SummaryFactory{
		baseCfg: cfg,
		store:   store,
		cache:   make(map[uuid.UUID]providers.Summarizer),
	}
}

// SummarizerForModel resolves provider settings by model ID and returns a Summarizer.
// Returns (nil, nil) when no summary_model is configured for the model's provider
// profile — callers should fall back to a static summarizer in that case.
func (f *SummaryFactory) SummarizerForModel(ctx context.Context, modelID uuid.UUID) (providers.Summarizer, error) {
	if f == nil || f.store == nil {
		return nil, fmt.Errorf("summary factory: model store is not configured")
	}
	f.mu.Lock()
	cached := f.cache[modelID]
	f.mu.Unlock()
	if cached != nil {
		return cached, nil
	}
	model, err := f.store.GetModelConfig(ctx, modelID)
	if err != nil {
		return nil, fmt.Errorf("summary factory: load model %s: %w", modelID, err)
	}
	if model == nil {
		return nil, fmt.Errorf("summary factory: model %s not found", modelID)
	}
	sum, err := newSummarizerForConfig(f.baseCfg, model)
	if err != nil {
		return nil, err
	}
	if sum == nil {
		return nil, nil
	}
	f.mu.Lock()
	f.cache[modelID] = sum
	f.mu.Unlock()
	return sum, nil
}

func newEmbedderForConfig(baseCfg *config.EmbeddingConfig, model *modelstore.ModelConfig) (providers.Embedder, error) {
	provider, serviceURL, apiKey, _, providerModel, err := resolveProviderConfig(baseCfg, model)
	if err != nil {
		return nil, err
	}
	cfg := *baseCfg
	switch provider {
	case "ollama":
		return providers.NewOllamaEmbedder(&cfg, providerModel, serviceURL), nil
	case "openai":
		baseURL := serviceURL
		if baseURL == "" {
			baseURL = defaultOpenAIBaseURL
		}
		if apiKey == "" && baseURL == defaultOpenAIBaseURL {
			return nil, fmt.Errorf("embedding factory: api_key is required for default OpenAI URL")
		}
		return providers.NewOpenAIEmbedder(&cfg, providerModel, baseURL, apiKey), nil
	default:
		return nil, fmt.Errorf("embedding factory: unsupported provider %q", provider)
	}
}

func newSummarizerForConfig(baseCfg *config.EmbeddingConfig, model *modelstore.ModelConfig) (providers.Summarizer, error) {
	provider, serviceURL, apiKey, summaryModel, _, err := resolveProviderConfig(baseCfg, model)
	if err != nil {
		return nil, err
	}
	if summaryModel == "" {
		// No summary model configured for this profile — caller should use fallback.
		return nil, nil
	}
	cfg := *baseCfg
	switch provider {
	case "ollama":
		return providers.NewOllamaSummarizer(&cfg, summaryModel, serviceURL), nil
	case "openai":
		baseURL := serviceURL
		if baseURL == "" {
			baseURL = defaultOpenAIBaseURL
		}
		if apiKey == "" && baseURL == defaultOpenAIBaseURL {
			return nil, fmt.Errorf("summary factory: api_key is required for default OpenAI URL")
		}
		return providers.NewOpenAISummarizer(&cfg, summaryModel, baseURL, apiKey), nil
	default:
		return nil, fmt.Errorf("summary factory: unsupported provider %q", provider)
	}
}

// resolveProviderConfig derives runtime provider settings from a model's stored
// config and the base embedding config profiles. Return order:
// provider, serviceURL, apiKey, summaryModel, providerModel, error.
func resolveProviderConfig(baseCfg *config.EmbeddingConfig, model *modelstore.ModelConfig) (string, string, string, string, string, error) {
	profileName := strings.TrimSpace(model.ProviderConfig)
	if profileName == "" {
		profileName = "default"
	}
	resolvedProvider := strings.TrimSpace(model.Provider)
	resolvedServiceURL := strings.TrimSpace(model.ServiceURL)
	resolvedAPIKey := ""
	resolvedSummaryModel := ""
	if profile, ok := baseCfg.Providers[profileName]; ok {
		// provider (backend type) is authoritative from the DB — the profile must
		// not override it, because the model was registered for a specific backend
		// and changing the profile would silently reroute it to a different API.
		// The profile supplies credentials (api_key) and optional service_url
		// overrides that are not persisted in the DB.
		if resolvedProvider == "" && strings.TrimSpace(profile.Backend) != "" {
			resolvedProvider = strings.TrimSpace(profile.Backend)
		}
		if resolvedServiceURL == "" && strings.TrimSpace(profile.ServiceURL) != "" {
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
	providerModel := strings.TrimSpace(model.ProviderModel)
	if providerModel == "" {
		return "", "", "", "", "", fmt.Errorf("embedding factory: model %s has empty provider_model", model.ID)
	}
	provider := strings.ToLower(strings.TrimSpace(resolvedProvider))
	return provider, resolvedServiceURL, resolvedAPIKey, resolvedSummaryModel, providerModel, nil
}
