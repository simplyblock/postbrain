package embedding

import (
	"context"
	"fmt"

	"github.com/simplyblock/postbrain/internal/config"
)

// EmbeddingService wraps a text embedder, an optional code embedder, and an
// optional text-generation summarizer.
type EmbeddingService struct {
	text       Embedder
	code       Embedder   // may be nil if no code model is configured
	summarizer Summarizer // may be nil if no summary model is configured
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
		svc := &EmbeddingService{
			text: NewOpenAIEmbedder(cfg, cfg.TextModel, ""),
		}
		if cfg.CodeModel != "" {
			svc.code = NewOpenAIEmbedder(cfg, cfg.CodeModel, "")
		}
		if cfg.SummaryModel != "" {
			svc.summarizer = NewOpenAISummarizer(cfg, cfg.SummaryModel, "")
		}
		return svc, nil

	default:
		return nil, fmt.Errorf("unsupported embedding backend: %s", cfg.Backend)
	}
}

// EmbedText embeds text using the text model.
func (s *EmbeddingService) EmbedText(ctx context.Context, text string) ([]float32, error) {
	return s.text.Embed(ctx, text)
}

// EmbedCode embeds text using the code model.
// If no code model is configured it falls back to the text model.
func (s *EmbeddingService) EmbedCode(ctx context.Context, text string) ([]float32, error) {
	if s.code != nil {
		return s.code.Embed(ctx, text)
	}
	return s.text.Embed(ctx, text)
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
