package embedding

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"sync"

	"github.com/simplyblock/postbrain/internal/config"
)

const defaultOpenAIBaseURL = "https://api.openai.com/v1"

// OpenAIEmbedder calls the OpenAI embeddings API to produce embeddings.
type OpenAIEmbedder struct {
	cfg       *config.EmbeddingConfig
	modelSlug string
	baseURL   string // overrideable for tests

	mu   sync.Mutex
	dims int // -1 until first successful embed
}

// NewOpenAIEmbedder creates an OpenAIEmbedder for the given model.
// If baseURL is non-empty it overrides the default OpenAI API base URL
// (useful for httptest servers in tests).
func NewOpenAIEmbedder(cfg *config.EmbeddingConfig, modelSlug string, baseURL string) *OpenAIEmbedder {
	if baseURL == "" {
		baseURL = defaultOpenAIBaseURL
	}
	return &OpenAIEmbedder{
		cfg:       cfg,
		modelSlug: modelSlug,
		baseURL:   baseURL,
		dims:      -1,
	}
}

// ModelSlug returns the model identifier.
func (e *OpenAIEmbedder) ModelSlug() string { return e.modelSlug }

// Dimensions returns the embedding dimension, or -1 if not yet determined.
func (e *OpenAIEmbedder) Dimensions() int {
	e.mu.Lock()
	defer e.mu.Unlock()
	return e.dims
}

// openAIRequest is the JSON body sent to OpenAI.
type openAIRequest struct {
	Model string   `json:"model"`
	Input []string `json:"input"`
}

// openAIEmbedData is a single item in the OpenAI response data array.
type openAIEmbedData struct {
	Embedding []float32 `json:"embedding"`
	Index     int       `json:"index"`
}

// openAIResponse is the JSON body returned by OpenAI.
type openAIResponse struct {
	Data []openAIEmbedData `json:"data"`
}

// Embed embeds a single text by delegating to EmbedBatch.
func (e *OpenAIEmbedder) Embed(ctx context.Context, text string) ([]float32, error) {
	results, err := e.EmbedBatch(ctx, []string{text})
	if err != nil {
		return nil, err
	}
	if len(results) == 0 {
		return nil, fmt.Errorf("openai: no embedding returned")
	}
	return results[0], nil
}

// EmbedBatch embeds multiple texts, chunking into batches of cfg.BatchSize.
func (e *OpenAIEmbedder) EmbedBatch(ctx context.Context, texts []string) ([][]float32, error) {
	batchSize := e.cfg.BatchSize
	if batchSize <= 0 {
		batchSize = 64
	}

	results := make([][]float32, len(texts))
	for start := 0; start < len(texts); start += batchSize {
		end := start + batchSize
		if end > len(texts) {
			end = len(texts)
		}
		batch := texts[start:end]

		embeddings, err := e.embedBatchOnce(ctx, batch)
		if err != nil {
			return nil, fmt.Errorf("openai: embed batch [%d:%d]: %w", start, end, err)
		}
		for i, emb := range embeddings {
			results[start+i] = emb
		}
	}
	return results, nil
}

// openAIMaxBytes is a conservative byte-length guard (≈8 000 tokens × ~4 bytes/token).
// The real limit is token-based but a byte check catches obviously oversized inputs
// before they hit the API and return an opaque 400.
const openAIMaxBytes = 32_000

// embedBatchOnce makes a single POST request to the OpenAI embeddings endpoint.
func (e *OpenAIEmbedder) embedBatchOnce(ctx context.Context, texts []string) ([][]float32, error) {
	for i, t := range texts {
		if t == "" {
			return nil, fmt.Errorf("openai: input[%d] is empty", i)
		}
		if len(t) > openAIMaxBytes {
			return nil, fmt.Errorf("openai: input[%d] too long (%d bytes, max ~%d)", i, len(t), openAIMaxBytes)
		}
	}
	if e.cfg.RequestTimeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, e.cfg.RequestTimeout)
		defer cancel()
	}

	body, err := json.Marshal(openAIRequest{Model: e.modelSlug, Input: texts})
	if err != nil {
		return nil, fmt.Errorf("openai: marshal request: %w", err)
	}

	url := e.baseURL + "/embeddings"
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("openai: create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+e.cfg.OpenAIAPIKey)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("openai: do request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		errBody, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
		return nil, fmt.Errorf("openai: unexpected status %d: %s", resp.StatusCode, bytes.TrimSpace(errBody))
	}

	var result openAIResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("openai: decode response: %w", err)
	}

	if len(result.Data) != len(texts) {
		return nil, fmt.Errorf("openai: expected %d embeddings, got %d", len(texts), len(result.Data))
	}

	// OpenAI may return data in any order; sort by index to restore original order.
	ordered := make([][]float32, len(texts))
	for _, item := range result.Data {
		if item.Index < 0 || item.Index >= len(texts) {
			return nil, fmt.Errorf("openai: embedding index %d out of range", item.Index)
		}
		ordered[item.Index] = item.Embedding
	}

	if len(ordered) > 0 {
		e.mu.Lock()
		if len(ordered[0]) > 0 {
			e.dims = len(ordered[0])
		}
		e.mu.Unlock()
	}

	return ordered, nil
}
