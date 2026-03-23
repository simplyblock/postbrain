package embedding

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/simplyblock/postbrain/internal/config"
)

func TestOllamaSummarizer_Summarize(t *testing.T) {
	t.Parallel()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/generate" {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		var req struct {
			Model  string `json:"model"`
			Prompt string `json:"prompt"`
			Stream bool   `json:"stream"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Errorf("decode body: %v", err)
		}
		if req.Stream {
			t.Error("stream should be false")
		}
		if !strings.Contains(req.Prompt, "document") {
			t.Errorf("expected prompt to contain document content, got: %q", req.Prompt)
		}
		json.NewEncoder(w).Encode(map[string]any{"response": "This is the summary."})
	}))
	defer srv.Close()

	cfg := &config.EmbeddingConfig{OllamaURL: srv.URL}
	s := NewOllamaSummarizer(cfg, "llama3")
	got, err := s.Summarize(context.Background(), "This is the document content.")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != "This is the summary." {
		t.Errorf("expected summary, got %q", got)
	}
}

func TestOpenAISummarizer_Summarize(t *testing.T) {
	t.Parallel()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/chat/completions" {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		var req struct {
			Model    string `json:"model"`
			Messages []struct {
				Role    string `json:"role"`
				Content string `json:"content"`
			} `json:"messages"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Errorf("decode body: %v", err)
		}
		if len(req.Messages) == 0 {
			t.Error("expected at least one message")
		}
		json.NewEncoder(w).Encode(map[string]any{
			"choices": []map[string]any{
				{"message": map[string]any{"content": "OpenAI summary."}},
			},
		})
	}))
	defer srv.Close()

	cfg := &config.EmbeddingConfig{OpenAIAPIKey: "test-key"}
	s := NewOpenAISummarizer(cfg, "gpt-4o-mini", srv.URL)
	got, err := s.Summarize(context.Background(), "Document text here.")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != "OpenAI summary." {
		t.Errorf("expected summary, got %q", got)
	}
}

func TestEmbeddingService_SummarizeNoModel(t *testing.T) {
	t.Parallel()
	svc := &EmbeddingService{} // no summarizer configured
	got, err := svc.Summarize(context.Background(), "Some text.")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != "" {
		t.Errorf("expected empty string when no summarizer configured, got %q", got)
	}
}

func TestEmbeddingService_SummarizeDelegates(t *testing.T) {
	t.Parallel()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(map[string]any{"response": "delegated summary"})
	}))
	defer srv.Close()

	cfg := &config.EmbeddingConfig{OllamaURL: srv.URL}
	svc := &EmbeddingService{
		summarizer: NewOllamaSummarizer(cfg, "llama3"),
	}
	got, err := svc.Summarize(context.Background(), "text")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != "delegated summary" {
		t.Errorf("got %q", got)
	}
}
