package providers

import (
	"context"
	"fmt"

	"github.com/google/uuid"
	"github.com/simplyblock/postbrain/internal/config"
)

// EmbeddingService wraps a text embedder, an optional code embedder, and an
// optional text-generation summarizer.
type EmbeddingService struct {
	text       Embedder
	code       Embedder   // may be nil if no code model is configured
	summarizer Summarizer // may be nil if no summary model is configured

	embedFactory         EmbedderResolver
	sumFactory           SummarizerResolver
	activeTextModelID    *uuid.UUID
	activeCodeModelID    *uuid.UUID
	activeSummaryModelID *uuid.UUID
}

// EmbedderResolver creates Embedder instances by model ID.
type EmbedderResolver interface {
	EmbedderForModel(ctx context.Context, modelID uuid.UUID) (Embedder, error)
}

// SummarizerResolver creates Summarizer instances by model ID.
// SummarizerForModel returns (nil, nil) when no summary model is configured for
// the given model ID — callers should fall back to a static summarizer.
type SummarizerResolver interface {
	SummarizerForModel(ctx context.Context, modelID uuid.UUID) (Summarizer, error)
}

// EmbedResult carries both embedding bytes and the model identity.
type EmbedResult struct {
	ModelID   uuid.UUID
	Embedding []float32
}

// NewService constructs an EmbeddingService from the given configuration.
// It returns an error if the backend is not supported.
func NewService(cfg *config.EmbeddingConfig) (*EmbeddingService, error) {
	provider := startupProvider(cfg)
	switch provider.Backend {
	case "ollama":
		effective := *cfg
		svc := &EmbeddingService{
			text: NewOllamaEmbedder(&effective, provider.TextModel, provider.ServiceURL),
		}
		if provider.CodeModel != "" {
			svc.code = NewOllamaEmbedder(&effective, provider.CodeModel, provider.ServiceURL)
		}
		if provider.SummaryModel != "" {
			svc.summarizer = NewOllamaSummarizer(&effective, provider.SummaryModel, provider.ServiceURL)
		}
		return svc, nil

	case "openai":
		effective := *cfg
		if provider.APIKey == "" && provider.ServiceURL == "" {
			return nil, fmt.Errorf("embedding.providers.default.api_key is required when service_url is not set")
		}
		baseURL := serviceURLOrDefault(provider.ServiceURL, defaultOpenAIBaseURL)
		svc := &EmbeddingService{
			text: NewOpenAIEmbedder(&effective, provider.TextModel, baseURL, provider.APIKey),
		}
		if provider.CodeModel != "" {
			svc.code = NewOpenAIEmbedder(&effective, provider.CodeModel, baseURL, provider.APIKey)
		}
		if provider.SummaryModel != "" {
			svc.summarizer = NewOpenAISummarizer(&effective, provider.SummaryModel, baseURL, provider.APIKey)
		}
		return svc, nil

	default:
		return nil, fmt.Errorf("unsupported embedding backend: %s", provider.Backend)
	}
}

func startupProvider(cfg *config.EmbeddingConfig) config.EmbeddingProviderConfig {
	if cfg == nil {
		return config.EmbeddingProviderConfig{
			Backend:      "ollama",
			TextModel:    "nomic-embed-text",
			CodeModel:    "nomic-embed-code",
			SummaryModel: "",
		}
	}
	p := cfg.Providers["default"]
	if p.Backend == "" {
		p.Backend = "ollama"
	}
	if p.TextModel == "" {
		p.TextModel = "nomic-embed-text"
	}
	return p
}

// EmbedText embeds text using the text model.
func (s *EmbeddingService) EmbedText(ctx context.Context, text string) ([]float32, error) {
	res, err := s.EmbedTextResult(ctx, text)
	if err != nil {
		return nil, err
	}
	return res.Embedding, nil
}

// EmbedCode embeds text using the code model.
// If no code model is configured it falls back to the text model.
func (s *EmbeddingService) EmbedCode(ctx context.Context, text string) ([]float32, error) {
	res, err := s.EmbedCodeResult(ctx, text)
	if err != nil {
		return nil, err
	}
	return res.Embedding, nil
}

// EmbedTextResult embeds text and also reports the model ID when model-aware
// factory resolution is configured.
func (s *EmbeddingService) EmbedTextResult(ctx context.Context, text string) (*EmbedResult, error) {
	if s.embedFactory != nil && s.activeTextModelID != nil {
		emb, err := s.embedFactory.EmbedderForModel(ctx, *s.activeTextModelID)
		if err != nil {
			return nil, err
		}
		vec, err := emb.Embed(ctx, text)
		if err != nil {
			return nil, err
		}
		return &EmbedResult{ModelID: *s.activeTextModelID, Embedding: vec}, nil
	}

	vec, err := s.text.Embed(ctx, text)
	if err != nil {
		return nil, err
	}
	return &EmbedResult{Embedding: vec}, nil
}

// EmbedCodeResult embeds code and also reports the model ID when model-aware
// factory resolution is configured.
func (s *EmbeddingService) EmbedCodeResult(ctx context.Context, text string) (*EmbedResult, error) {
	if s.embedFactory != nil && s.activeCodeModelID != nil {
		emb, err := s.embedFactory.EmbedderForModel(ctx, *s.activeCodeModelID)
		if err != nil {
			return nil, err
		}
		vec, err := emb.Embed(ctx, text)
		if err != nil {
			return nil, err
		}
		return &EmbedResult{ModelID: *s.activeCodeModelID, Embedding: vec}, nil
	}
	if s.code != nil {
		vec, err := s.code.Embed(ctx, text)
		if err != nil {
			return nil, err
		}
		return &EmbedResult{Embedding: vec}, nil
	}
	vec, err := s.text.Embed(ctx, text)
	if err != nil {
		return nil, err
	}
	return &EmbedResult{Embedding: vec}, nil
}

// Summarize generates a text summary using the configured summary model.
// Returns an empty string (and no error) when no summary model is configured,
// allowing callers to fall back to extractive summarization.
func (s *EmbeddingService) Summarize(ctx context.Context, text string) (string, error) {
	if s.sumFactory != nil && s.activeSummaryModelID != nil {
		sum, err := s.sumFactory.SummarizerForModel(ctx, *s.activeSummaryModelID)
		if err != nil {
			return "", err
		}
		if sum != nil {
			return sum.Summarize(ctx, text)
		}
		// sum == nil: no summary model configured; fall through to static summarizer.
	}
	if s.summarizer == nil {
		return "", nil
	}
	return s.summarizer.Summarize(ctx, text)
}

// Analyze performs a combined summarize+entity-extraction call using the
// configured summary model. Returns nil, nil when no summary model is configured,
// allowing callers to fall back to heuristic extraction.
func (s *EmbeddingService) Analyze(ctx context.Context, text string) (*DocumentAnalysis, error) {
	if s.sumFactory != nil && s.activeSummaryModelID != nil {
		sum, err := s.sumFactory.SummarizerForModel(ctx, *s.activeSummaryModelID)
		if err != nil {
			return nil, err
		}
		if sum != nil {
			return sum.Analyze(ctx, text)
		}
		// sum == nil: no summary model configured; fall through to static summarizer.
	}
	if s.summarizer == nil {
		return nil, nil
	}
	return s.summarizer.Analyze(ctx, text)
}

// TextEmbedder returns the underlying text Embedder.
func (s *EmbeddingService) TextEmbedder() Embedder { return s.text }

// CodeEmbedder returns the underlying code Embedder, or nil if none is configured.
func (s *EmbeddingService) CodeEmbedder() Embedder { return s.code }

// NewServiceFromEmbedders constructs an EmbeddingService directly from Embedder instances.
// code may be nil; in that case EmbedCode falls back to the text embedder.
// This constructor is primarily intended for testing.
func NewServiceFromEmbedders(text Embedder, code Embedder) *EmbeddingService {
	return &EmbeddingService{text: text, code: code}
}

// SetModelFactory configures optional model-aware embedder and summarizer resolution.
func (s *EmbeddingService) SetModelFactory(embedFactory EmbedderResolver, sumFactory SummarizerResolver, activeTextModelID, activeCodeModelID, activeSummaryModelID *uuid.UUID) {
	s.embedFactory = embedFactory
	s.sumFactory = sumFactory
	s.activeTextModelID = activeTextModelID
	s.activeCodeModelID = activeCodeModelID
	s.activeSummaryModelID = activeSummaryModelID
}
