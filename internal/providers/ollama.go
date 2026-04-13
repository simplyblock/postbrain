package providers

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"sync"

	"github.com/simplyblock/postbrain/internal/closeutil"
	"github.com/simplyblock/postbrain/internal/config"
)

// OllamaEmbedder calls the Ollama HTTP API to produce embeddings.
type OllamaEmbedder struct {
	cfg        *config.EmbeddingConfig
	modelSlug  string
	serviceURL string

	mu   sync.Mutex
	dims int // -1 until first successful embed
}

// NewOllamaEmbedder creates an OllamaEmbedder for the given model.
func NewOllamaEmbedder(cfg *config.EmbeddingConfig, modelSlug string, serviceURL string) *OllamaEmbedder {
	return &OllamaEmbedder{
		cfg:        cfg,
		modelSlug:  modelSlug,
		serviceURL: serviceURL,
		dims:       -1,
	}
}

// ModelSlug returns the model identifier.
func (e *OllamaEmbedder) ModelSlug() string { return e.modelSlug }

// ServiceURL returns the configured service URL (may be empty, meaning default is used).
func (e *OllamaEmbedder) ServiceURL() string { return e.serviceURL }

// Dimensions returns the embedding dimension, or -1 if not yet determined.
func (e *OllamaEmbedder) Dimensions() int {
	e.mu.Lock()
	defer e.mu.Unlock()
	return e.dims
}

// ollamaRequest is the JSON body sent to Ollama.
type ollamaRequest struct {
	Model  string `json:"model"`
	Prompt string `json:"prompt"`
}

// ollamaResponse is the JSON body returned by Ollama.
type ollamaResponse struct {
	Embedding []float32 `json:"embedding"`
}

// Embed embeds a single text via the Ollama API.
func (e *OllamaEmbedder) Embed(ctx context.Context, text string) ([]float32, error) {
	if e.cfg.RequestTimeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, e.cfg.RequestTimeout)
		defer cancel()
	}

	body, err := json.Marshal(ollamaRequest{Model: e.modelSlug, Prompt: text})
	if err != nil {
		return nil, fmt.Errorf("ollama: marshal request: %w", err)
	}

	url := serviceURLOrDefault(e.serviceURL, defaultOllamaServiceURL) + "/api/embeddings"
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("ollama: create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("ollama: do request: %w", err)
	}
	defer closeutil.Log(resp.Body, "ollama embedding response body")

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("ollama: unexpected status %d", resp.StatusCode)
	}

	var result ollamaResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("ollama: decode response: %w", err)
	}

	if len(result.Embedding) == 0 {
		return nil, fmt.Errorf("ollama: empty embedding returned for model %q", e.modelSlug)
	}

	e.mu.Lock()
	e.dims = len(result.Embedding)
	e.mu.Unlock()

	return result.Embedding, nil
}

// EmbedBatch embeds multiple texts sequentially (Ollama has no batch endpoint).
func (e *OllamaEmbedder) EmbedBatch(ctx context.Context, texts []string) ([][]float32, error) {
	results := make([][]float32, len(texts))
	for i, text := range texts {
		emb, err := e.Embed(ctx, text)
		if err != nil {
			return nil, fmt.Errorf("ollama: embed batch item %d: %w", i, err)
		}
		results[i] = emb
	}
	return results, nil
}
