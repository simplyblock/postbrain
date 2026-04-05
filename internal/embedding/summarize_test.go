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
		if err := json.NewEncoder(w).Encode(map[string]any{"response": "This is the summary."}); err != nil {
			t.Errorf("encode response: %v", err)
		}
	}))
	defer srv.Close()

	cfg := &config.EmbeddingConfig{}
	s := NewOllamaSummarizer(cfg, "llama3", srv.URL)
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
		if r.URL.Path != "/chat/completions" {
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
		if err := json.NewEncoder(w).Encode(map[string]any{
			"choices": []map[string]any{
				{"message": map[string]any{"content": "OpenAI summary."}},
			},
		}); err != nil {
			t.Errorf("encode response: %v", err)
		}
	}))
	defer srv.Close()

	cfg := &config.EmbeddingConfig{}
	s := NewOpenAISummarizer(cfg, "gpt-4o-mini", srv.URL, "test-key")
	got, err := s.Summarize(context.Background(), "Document text here.")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != "OpenAI summary." {
		t.Errorf("expected summary, got %q", got)
	}
}

func TestOpenAISummarizer_CustomBaseURL_EmptyAPIKeyOmitsAuthorizationHeader(t *testing.T) {
	t.Parallel()
	seenAuth := "unset"
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		seenAuth = r.Header.Get("Authorization")
		if err := json.NewEncoder(w).Encode(map[string]any{
			"choices": []map[string]any{
				{"message": map[string]any{"content": "OpenAI summary."}},
			},
		}); err != nil {
			t.Errorf("encode response: %v", err)
		}
	}))
	defer srv.Close()

	cfg := &config.EmbeddingConfig{}
	s := NewOpenAISummarizer(cfg, "gpt-4o-mini", srv.URL, "")
	got, err := s.Summarize(context.Background(), "Document text here.")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != "OpenAI summary." {
		t.Errorf("expected summary, got %q", got)
	}
	if seenAuth != "" {
		t.Fatalf("Authorization header = %q, want empty", seenAuth)
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
		if err := json.NewEncoder(w).Encode(map[string]any{"response": "delegated summary"}); err != nil {
			t.Errorf("encode response: %v", err)
		}
	}))
	defer srv.Close()

	cfg := &config.EmbeddingConfig{}
	svc := &EmbeddingService{
		summarizer: NewOllamaSummarizer(cfg, "llama3", srv.URL),
	}
	got, err := svc.Summarize(context.Background(), "text")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != "delegated summary" {
		t.Errorf("got %q", got)
	}
}

func TestOllamaSummarizer_Analyze(t *testing.T) {
	t.Parallel()
	analysis := map[string]any{
		"summary": "A document about authentication.",
		"entities": []map[string]any{
			{"type": "concept", "name": "Authentication", "canonical": "authentication"},
			{"type": "technology", "name": "PostgreSQL", "canonical": "postgresql"},
			{"type": "tag", "name": "security", "canonical": "security"},
		},
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			Format string `json:"format"`
			Prompt string `json:"prompt"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Errorf("decode body: %v", err)
		}
		if req.Format != "json" {
			t.Errorf("expected format=json, got %q", req.Format)
		}
		if !strings.Contains(req.Prompt, "entities") {
			t.Errorf("expected analyze prompt, got: %q", req.Prompt)
		}
		raw, err := json.Marshal(analysis)
		if err != nil {
			t.Errorf("marshal analysis: %v", err)
			return
		}
		if err := json.NewEncoder(w).Encode(map[string]any{"response": string(raw)}); err != nil {
			t.Errorf("encode response: %v", err)
		}
	}))
	defer srv.Close()

	cfg := &config.EmbeddingConfig{}
	s := NewOllamaSummarizer(cfg, "llama3", srv.URL)
	got, err := s.Analyze(context.Background(), "Document about auth and PostgreSQL.")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got.Summary != "A document about authentication." {
		t.Errorf("unexpected summary: %q", got.Summary)
	}
	if len(got.Entities) != 3 {
		t.Errorf("expected 3 entities, got %d", len(got.Entities))
	}
	if got.Entities[0].Type != "concept" || got.Entities[0].Canonical != "authentication" {
		t.Errorf("unexpected first entity: %+v", got.Entities[0])
	}
}

func TestOpenAISummarizer_Analyze(t *testing.T) {
	t.Parallel()
	analysis := map[string]any{
		"summary": "An article about distributed systems.",
		"entities": []map[string]any{
			{"type": "topic", "name": "Distributed Systems", "canonical": "distributed_systems"},
			{"type": "technology", "name": "Kafka", "canonical": "kafka"},
			{"type": "person", "name": "Leslie Lamport", "canonical": "leslie_lamport"},
		},
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			ResponseFormat map[string]string `json:"response_format"`
			Messages       []struct {
				Content string `json:"content"`
			} `json:"messages"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Errorf("decode body: %v", err)
		}
		if req.ResponseFormat["type"] != "json_object" {
			t.Errorf("expected response_format json_object, got %v", req.ResponseFormat)
		}
		if len(req.Messages) == 0 || !strings.Contains(req.Messages[0].Content, "entities") {
			t.Errorf("expected analyze prompt in message")
		}
		raw, err := json.Marshal(analysis)
		if err != nil {
			t.Errorf("marshal analysis: %v", err)
			return
		}
		if err := json.NewEncoder(w).Encode(map[string]any{
			"choices": []map[string]any{
				{"message": map[string]any{"content": string(raw)}},
			},
		}); err != nil {
			t.Errorf("encode response: %v", err)
		}
	}))
	defer srv.Close()

	cfg := &config.EmbeddingConfig{}
	s := NewOpenAISummarizer(cfg, "gpt-4o-mini", srv.URL, "test-key")
	got, err := s.Analyze(context.Background(), "Article about distributed systems.")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got.Summary != "An article about distributed systems." {
		t.Errorf("unexpected summary: %q", got.Summary)
	}
	if len(got.Entities) != 3 {
		t.Errorf("expected 3 entities, got %d", len(got.Entities))
	}
	if got.Entities[2].Type != "person" || got.Entities[2].Canonical != "leslie_lamport" {
		t.Errorf("unexpected third entity: %+v", got.Entities[2])
	}
}

func TestEmbeddingService_AnalyzeNoModel(t *testing.T) {
	t.Parallel()
	svc := &EmbeddingService{} // no summarizer configured
	got, err := svc.Analyze(context.Background(), "Some text.")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != nil {
		t.Errorf("expected nil analysis when no summarizer configured, got %+v", got)
	}
}

func TestEmbeddingService_AnalyzeDelegates(t *testing.T) {
	t.Parallel()
	analysis := map[string]any{
		"summary":  "Delegated analysis.",
		"entities": []map[string]any{{"type": "tag", "name": "test", "canonical": "test"}},
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		raw, err := json.Marshal(analysis)
		if err != nil {
			t.Errorf("marshal analysis: %v", err)
			return
		}
		if err := json.NewEncoder(w).Encode(map[string]any{"response": string(raw)}); err != nil {
			t.Errorf("encode response: %v", err)
		}
	}))
	defer srv.Close()

	cfg := &config.EmbeddingConfig{}
	svc := &EmbeddingService{
		summarizer: NewOllamaSummarizer(cfg, "llama3", srv.URL),
	}
	got, err := svc.Analyze(context.Background(), "text")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got == nil || got.Summary != "Delegated analysis." {
		t.Errorf("unexpected result: %+v", got)
	}
}
