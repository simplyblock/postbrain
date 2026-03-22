package embedding

import (
	"context"
	"fmt"

	"github.com/simplyblock/postbrain/internal/config"
)

// EmbeddingService wraps a text embedder and an optional code embedder.
type EmbeddingService struct {
	text Embedder
	code Embedder // may be nil if no code model is configured
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
		return svc, nil

	case "openai":
		svc := &EmbeddingService{
			text: NewOpenAIEmbedder(cfg, cfg.TextModel, ""),
		}
		if cfg.CodeModel != "" {
			svc.code = NewOpenAIEmbedder(cfg, cfg.CodeModel, "")
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

// TextEmbedder returns the underlying text Embedder.
func (s *EmbeddingService) TextEmbedder() Embedder { return s.text }

// CodeEmbedder returns the underlying code Embedder, or nil if none is configured.
func (s *EmbeddingService) CodeEmbedder() Embedder { return s.code }
