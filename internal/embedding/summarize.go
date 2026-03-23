package embedding

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/simplyblock/postbrain/internal/config"
)

const summarizePrompt = "Summarize the following document in 3-5 sentences, capturing the main topic and key points. Output only the summary text with no preamble.\n\nDocument:\n"

// Summarizer generates a human-readable text summary of a document.
type Summarizer interface {
	Summarize(ctx context.Context, text string) (string, error)
}

// OllamaSummarizer calls the Ollama /api/generate endpoint to produce summaries.
type OllamaSummarizer struct {
	cfg       *config.EmbeddingConfig
	modelSlug string
}

// NewOllamaSummarizer creates an OllamaSummarizer for the given model.
func NewOllamaSummarizer(cfg *config.EmbeddingConfig, modelSlug string) *OllamaSummarizer {
	return &OllamaSummarizer{cfg: cfg, modelSlug: modelSlug}
}

// Summarize sends the document to Ollama and returns the generated summary.
func (s *OllamaSummarizer) Summarize(ctx context.Context, text string) (string, error) {
	if s.cfg.RequestTimeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, s.cfg.RequestTimeout)
		defer cancel()
	}

	body, err := json.Marshal(map[string]any{
		"model":  s.modelSlug,
		"prompt": summarizePrompt + text,
		"stream": false,
	})
	if err != nil {
		return "", fmt.Errorf("ollama summarize: marshal: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost,
		s.cfg.OllamaURL+"/api/generate", bytes.NewReader(body))
	if err != nil {
		return "", fmt.Errorf("ollama summarize: build request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("ollama summarize: do request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("ollama summarize: unexpected status %d", resp.StatusCode)
	}

	var result struct {
		Response string `json:"response"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return "", fmt.Errorf("ollama summarize: decode response: %w", err)
	}
	return result.Response, nil
}

// OpenAISummarizer calls the OpenAI-compatible /v1/chat/completions endpoint to produce summaries.
type OpenAISummarizer struct {
	cfg       *config.EmbeddingConfig
	modelSlug string
	baseURL   string
}

// NewOpenAISummarizer creates an OpenAISummarizer for the given model.
// If baseURL is empty the default OpenAI base URL is used.
func NewOpenAISummarizer(cfg *config.EmbeddingConfig, modelSlug string, baseURL string) *OpenAISummarizer {
	if baseURL == "" {
		baseURL = defaultOpenAIBaseURL
	}
	return &OpenAISummarizer{cfg: cfg, modelSlug: modelSlug, baseURL: baseURL}
}

// Summarize sends the document to the OpenAI chat completions endpoint and returns the summary.
func (s *OpenAISummarizer) Summarize(ctx context.Context, text string) (string, error) {
	if s.cfg.RequestTimeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, s.cfg.RequestTimeout)
		defer cancel()
	}

	body, err := json.Marshal(map[string]any{
		"model": s.modelSlug,
		"messages": []map[string]string{
			{"role": "user", "content": summarizePrompt + text},
		},
		"max_tokens": 500,
	})
	if err != nil {
		return "", fmt.Errorf("openai summarize: marshal: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost,
		s.baseURL+"/v1/chat/completions", bytes.NewReader(body))
	if err != nil {
		return "", fmt.Errorf("openai summarize: build request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+s.cfg.OpenAIAPIKey)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("openai summarize: do request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("openai summarize: unexpected status %d", resp.StatusCode)
	}

	var result struct {
		Choices []struct {
			Message struct {
				Content string `json:"content"`
			} `json:"message"`
		} `json:"choices"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return "", fmt.Errorf("openai summarize: decode response: %w", err)
	}
	if len(result.Choices) == 0 {
		return "", fmt.Errorf("openai summarize: no choices returned")
	}
	return result.Choices[0].Message.Content, nil
}
