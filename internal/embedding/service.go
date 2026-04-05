package embedding

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

	factory           modelEmbedderResolver
	activeTextModelID *uuid.UUID
	activeCodeModelID *uuid.UUID
}

type modelEmbedderResolver interface {
	EmbedderForModel(ctx context.Context, modelID uuid.UUID) (Embedder, error)
}

// EmbedResult carries both embedding bytes and the model identity.
type EmbedResult struct {
	ModelID   uuid.UUID
	Embedding []float32
}

// NewService constructs an EmbeddingService from the given configuration.
// It returns an error if the backend is not supported.
func NewService(cfg *config.EmbeddingConfig) (*EmbeddingService, error) {
	switch cfg.Backend {
	case "ollama":
		svc := &EmbeddingService{
			text: NewOllamaEmbedder(cfg, cfg.TextModel),
		}
		if cfg.CodeModel != "" {
			svc.code = NewOllamaEmbedder(cfg, cfg.CodeModel)
		}
		if cfg.SummaryModel != "" {
			svc.summarizer = NewOllamaSummarizer(cfg, cfg.SummaryModel)
		}
		return svc, nil

	case "openai":
		if cfg.OpenAIAPIKey == "" && cfg.ServiceURL == "" {
			return nil, fmt.Errorf("openai_api_key is required when embedding.service_url is not set")
		}
		baseURL := serviceURLOrDefault(cfg, defaultOpenAIBaseURL)
		svc := &EmbeddingService{
			text: NewOpenAIEmbedder(cfg, cfg.TextModel, baseURL),
		}
		if cfg.CodeModel != "" {
			svc.code = NewOpenAIEmbedder(cfg, cfg.CodeModel, baseURL)
		}
		if cfg.SummaryModel != "" {
			svc.summarizer = NewOpenAISummarizer(cfg, cfg.SummaryModel, baseURL)
		}
		return svc, nil

	default:
		return nil, fmt.Errorf("unsupported embedding backend: %s", cfg.Backend)
	}
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
	if s.factory != nil && s.activeTextModelID != nil {
		emb, err := s.factory.EmbedderForModel(ctx, *s.activeTextModelID)
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
	if s.factory != nil && s.activeCodeModelID != nil {
		emb, err := s.factory.EmbedderForModel(ctx, *s.activeCodeModelID)
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
	if s.summarizer == nil {
		return "", nil
	}
	return s.summarizer.Summarize(ctx, text)
}

// Analyze performs a combined summarize+entity-extraction call using the
// configured summary model. Returns nil, nil when no summary model is configured,
// allowing callers to fall back to heuristic extraction.
func (s *EmbeddingService) Analyze(ctx context.Context, text string) (*DocumentAnalysis, error) {
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

// SetModelFactory configures optional model-aware embedder resolution.
func (s *EmbeddingService) SetModelFactory(factory modelEmbedderResolver, activeTextModelID, activeCodeModelID *uuid.UUID) {
	s.factory = factory
	s.activeTextModelID = activeTextModelID
	s.activeCodeModelID = activeCodeModelID
}
