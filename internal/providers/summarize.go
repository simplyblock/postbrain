package providers

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/simplyblock/postbrain/internal/closeutil"
	"github.com/simplyblock/postbrain/internal/config"
)

const summarizePrompt = "Summarize the following document in 3-5 sentences, capturing the main topic and key points. Output only the summary text with no preamble.\n\nDocument:\n"

const analyzePrompt = `Analyze the following document and return a JSON object with exactly two fields:

"summary": a 3-5 sentence summary capturing the main topic and key points.

"entities": an array of up to 20 objects extracted from the document. Each object has:
  "type": one of concept | technology | topic | person | file | pr | tag
  "name": human-readable display name
  "canonical": lowercase normalized identifier (use underscores for spaces; for pr use "pr:N"; for file use the path as-is)

Output only raw JSON — no markdown, no code fences, no preamble.

Document:
`

// AnalysedEntity is a single entity extracted by the LLM during document analysis.
type AnalysedEntity struct {
	Type      string `json:"type"` // concept|technology|topic|person|file|pr|tag
	Name      string `json:"name"`
	Canonical string `json:"canonical"`
}

// DocumentAnalysis is the structured result of a combined summarize+extract LLM call.
type DocumentAnalysis struct {
	Summary  string           `json:"summary"`
	Entities []AnalysedEntity `json:"entities"`
}

// Summarizer generates a human-readable text summary of a document and can
// also perform a combined analysis that extracts entities alongside the summary.
type Summarizer interface {
	Summarize(ctx context.Context, text string) (string, error)
	Analyze(ctx context.Context, text string) (*DocumentAnalysis, error)
}

// OllamaSummarizer calls the Ollama /api/generate endpoint to produce summaries.
type OllamaSummarizer struct {
	cfg        *config.EmbeddingConfig
	modelSlug  string
	serviceURL string
}

// NewOllamaSummarizer creates an OllamaSummarizer for the given model.
func NewOllamaSummarizer(cfg *config.EmbeddingConfig, modelSlug string, serviceURL string) *OllamaSummarizer {
	return &OllamaSummarizer{cfg: cfg, modelSlug: modelSlug, serviceURL: serviceURL}
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
		serviceURLOrDefault(s.serviceURL, defaultOllamaServiceURL)+"/api/generate", bytes.NewReader(body))
	if err != nil {
		return "", fmt.Errorf("ollama summarize: build request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("ollama summarize: do request: %w", err)
	}
	defer closeutil.Log(resp.Body, "ollama summarize response body")

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

// Analyze sends the document to Ollama with JSON mode enabled and returns a
// combined summary and entity list.
func (s *OllamaSummarizer) Analyze(ctx context.Context, text string) (*DocumentAnalysis, error) {
	if s.cfg.RequestTimeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, s.cfg.RequestTimeout)
		defer cancel()
	}

	body, err := json.Marshal(map[string]any{
		"model":  s.modelSlug,
		"prompt": analyzePrompt + text,
		"stream": false,
		"format": "json",
	})
	if err != nil {
		return nil, fmt.Errorf("ollama analyze: marshal: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost,
		serviceURLOrDefault(s.serviceURL, defaultOllamaServiceURL)+"/api/generate", bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("ollama analyze: build request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("ollama analyze: do request: %w", err)
	}
	defer closeutil.Log(resp.Body, "ollama analyze response body")

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("ollama analyze: unexpected status %d", resp.StatusCode)
	}

	var result struct {
		Response string `json:"response"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("ollama analyze: decode response: %w", err)
	}

	var analysis DocumentAnalysis
	if err := json.Unmarshal([]byte(result.Response), &analysis); err != nil {
		return nil, fmt.Errorf("ollama analyze: parse JSON: %w", err)
	}
	return &analysis, nil
}

// OpenAISummarizer calls the OpenAI-compatible /v1/chat/completions endpoint to produce summaries.
type OpenAISummarizer struct {
	cfg       *config.EmbeddingConfig
	modelSlug string
	baseURL   string
	apiKey    string
}

// NewOpenAISummarizer creates an OpenAISummarizer for the given model.
// If baseURL is empty the default OpenAI base URL is used.
func NewOpenAISummarizer(cfg *config.EmbeddingConfig, modelSlug string, baseURL string, apiKey string) *OpenAISummarizer {
	if baseURL == "" {
		baseURL = defaultOpenAIBaseURL
	}
	return &OpenAISummarizer{cfg: cfg, modelSlug: modelSlug, baseURL: baseURL, apiKey: apiKey}
}

// BaseURL returns the configured API base URL.
func (s *OpenAISummarizer) BaseURL() string { return s.baseURL }

// ModelSlug returns the model identifier used for generation.
func (s *OpenAISummarizer) ModelSlug() string { return s.modelSlug }

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
		s.baseURL+"/chat/completions", bytes.NewReader(body))
	if err != nil {
		return "", fmt.Errorf("openai summarize: build request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	if s.apiKey != "" {
		req.Header.Set("Authorization", "Bearer "+s.apiKey)
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("openai summarize: do request: %w", err)
	}
	defer closeutil.Log(resp.Body, "openai summarize response body")

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

// Analyze sends the document to the OpenAI chat completions endpoint using
// JSON mode and returns a combined summary and entity list.
func (s *OpenAISummarizer) Analyze(ctx context.Context, text string) (*DocumentAnalysis, error) {
	if s.cfg.RequestTimeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, s.cfg.RequestTimeout)
		defer cancel()
	}

	body, err := json.Marshal(map[string]any{
		"model": s.modelSlug,
		"messages": []map[string]string{
			{"role": "user", "content": analyzePrompt + text},
		},
		"max_tokens":      1000,
		"response_format": map[string]string{"type": "json_object"},
	})
	if err != nil {
		return nil, fmt.Errorf("openai analyze: marshal: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost,
		s.baseURL+"/chat/completions", bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("openai analyze: build request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	if s.apiKey != "" {
		req.Header.Set("Authorization", "Bearer "+s.apiKey)
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("openai analyze: do request: %w", err)
	}
	defer closeutil.Log(resp.Body, "openai analyze response body")

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("openai analyze: unexpected status %d", resp.StatusCode)
	}

	var result struct {
		Choices []struct {
			Message struct {
				Content string `json:"content"`
			} `json:"message"`
		} `json:"choices"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("openai analyze: decode response: %w", err)
	}
	if len(result.Choices) == 0 {
		return nil, fmt.Errorf("openai analyze: no choices returned")
	}

	var analysis DocumentAnalysis
	if err := json.Unmarshal([]byte(result.Choices[0].Message.Content), &analysis); err != nil {
		return nil, fmt.Errorf("openai analyze: parse JSON: %w", err)
	}
	return &analysis, nil
}
