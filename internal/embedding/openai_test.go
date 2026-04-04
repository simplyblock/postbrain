package embedding

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/simplyblock/postbrain/internal/config"
)

func newOpenAICfg(apiKey string) *config.EmbeddingConfig {
	return &config.EmbeddingConfig{
		Backend:        "openai",
		TextModel:      "text-embedding-3-small",
		OpenAIAPIKey:   apiKey,
		RequestTimeout: 5 * time.Second,
		BatchSize:      2, // small for testing multi-batch behaviour
	}
}

// buildOpenAIHandler returns an HTTP handler that mimics the OpenAI embeddings
// endpoint. It records how many requests were made via the provided counter.
func buildOpenAIHandler(t *testing.T, requestCount *int) http.HandlerFunc {
	t.Helper()
	return func(w http.ResponseWriter, r *http.Request) {
		*requestCount++

		if r.Method != http.MethodPost {
			t.Errorf("expected POST, got %s", r.Method)
		}
		auth := r.Header.Get("Authorization")
		if auth == "" {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}

		var req struct {
			Model string   `json:"model"`
			Input []string `json:"input"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Errorf("decode body: %v", err)
			w.WriteHeader(http.StatusBadRequest)
			return
		}

		type embData struct {
			Embedding []float32 `json:"embedding"`
			Index     int       `json:"index"`
			Object    string    `json:"object"`
		}
		var data []embData
		for i, text := range req.Input {
			// Produce a deterministic embedding based on text length.
			data = append(data, embData{
				Embedding: []float32{float32(len(text)), float32(i)},
				Index:     i,
				Object:    "embedding",
			})
		}
		w.Header().Set("Content-Type", "application/json")
		if err := json.NewEncoder(w).Encode(map[string]interface{}{
			"data":  data,
			"model": req.Model,
		}); err != nil {
			t.Errorf("encode response: %v", err)
		}
	}
}

func TestOpenAIEmbedder_SingleEmbed(t *testing.T) {
	count := 0
	srv := httptest.NewServer(buildOpenAIHandler(t, &count))
	defer srv.Close()

	e := NewOpenAIEmbedder(newOpenAICfg("sk-test"), "text-embedding-3-small", srv.URL)
	got, err := e.Embed(context.Background(), "hello")
	if err != nil {
		t.Fatalf("Embed: %v", err)
	}
	// Embedding for "hello" (5 chars) should be [5, 0].
	if len(got) != 2 {
		t.Fatalf("len(got) = %d; want 2", len(got))
	}
	if got[0] != 5 {
		t.Errorf("got[0] = %v; want 5", got[0])
	}
}

func TestOpenAIEmbedder_BatchOf3(t *testing.T) {
	count := 0
	srv := httptest.NewServer(buildOpenAIHandler(t, &count))
	defer srv.Close()

	// BatchSize is 2, so 3 items should produce 2 requests.
	e := NewOpenAIEmbedder(newOpenAICfg("sk-test"), "text-embedding-3-small", srv.URL)
	texts := []string{"hello", "world", "foo"}
	got, err := e.EmbedBatch(context.Background(), texts)
	if err != nil {
		t.Fatalf("EmbedBatch: %v", err)
	}
	if len(got) != 3 {
		t.Fatalf("len(got) = %d; want 3", len(got))
	}
	// Check correct order: embeddings are keyed by original text.
	// "hello" = 5 chars, "world" = 5, "foo" = 3
	if got[0][0] != 5 {
		t.Errorf("got[0][0] = %v; want 5 (len of 'hello')", got[0][0])
	}
	if got[2][0] != 3 {
		t.Errorf("got[2][0] = %v; want 3 (len of 'foo')", got[2][0])
	}
}

func TestOpenAIEmbedder_LargeBatchMakesMultipleRequests(t *testing.T) {
	count := 0
	srv := httptest.NewServer(buildOpenAIHandler(t, &count))
	defer srv.Close()

	// BatchSize is 2; send 5 texts → expect 3 requests (2+2+1).
	e := NewOpenAIEmbedder(newOpenAICfg("sk-test"), "text-embedding-3-small", srv.URL)
	texts := []string{"a", "bb", "ccc", "dddd", "eeeee"}
	got, err := e.EmbedBatch(context.Background(), texts)
	if err != nil {
		t.Fatalf("EmbedBatch: %v", err)
	}
	if len(got) != 5 {
		t.Fatalf("len(got) = %d; want 5", len(got))
	}
	if count != 3 {
		t.Errorf("number of HTTP requests = %d; want 3", count)
	}
	// Verify order is preserved.
	expectedLens := []float32{1, 2, 3, 4, 5}
	for i, emb := range got {
		if emb[0] != expectedLens[i] {
			t.Errorf("got[%d][0] = %v; want %v", i, emb[0], expectedLens[i])
		}
	}
}

func TestOpenAIEmbedder_MissingAPIKeyReturns401(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") == "" || r.Header.Get("Authorization") == "Bearer " {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	// Empty API key → Authorization: Bearer  (empty bearer)
	e := NewOpenAIEmbedder(newOpenAICfg(""), "text-embedding-3-small", srv.URL)
	_, err := e.Embed(context.Background(), "hello")
	if err == nil {
		t.Fatal("expected error for 401 response, got nil")
	}
}

func TestOpenAIEmbedder_CustomBaseURL_EmptyAPIKeyOmitsAuthorizationHeader(t *testing.T) {
	seenAuth := "unset"
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		seenAuth = r.Header.Get("Authorization")
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"data": []map[string]any{
				{"embedding": []float32{1, 2}, "index": 0},
			},
		})
	}))
	defer srv.Close()

	cfg := newOpenAICfg("")
	cfg.OpenAIBaseURL = srv.URL
	e := NewOpenAIEmbedder(cfg, "text-embedding-3-small", cfg.OpenAIBaseURL)
	_, err := e.Embed(context.Background(), "hello")
	if err != nil {
		t.Fatalf("Embed: %v", err)
	}
	if seenAuth != "" {
		t.Fatalf("Authorization header = %q, want empty", seenAuth)
	}
}

func TestOpenAIEmbedder_ArrayResponse_SingleInput(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode([][]float32{{0.1, 0.2, 0.3}})
	}))
	defer srv.Close()

	cfg := newOpenAICfg("")
	cfg.OpenAIBaseURL = srv.URL
	e := NewOpenAIEmbedder(cfg, "text-embedding-3-small", cfg.OpenAIBaseURL)
	got, err := e.Embed(context.Background(), "hello")
	if err != nil {
		t.Fatalf("Embed: %v", err)
	}
	if len(got) != 3 || got[0] != 0.1 || got[1] != 0.2 || got[2] != 0.3 {
		t.Fatalf("unexpected embedding: %v", got)
	}
}

func TestOpenAIEmbedder_ArrayResponse_BatchInput(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode([][]float32{
			{1, 1},
			{2, 2},
		})
	}))
	defer srv.Close()

	cfg := newOpenAICfg("")
	cfg.OpenAIBaseURL = srv.URL
	e := NewOpenAIEmbedder(cfg, "text-embedding-3-small", cfg.OpenAIBaseURL)
	got, err := e.EmbedBatch(context.Background(), []string{"one", "two"})
	if err != nil {
		t.Fatalf("EmbedBatch: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("len(got)=%d, want 2", len(got))
	}
	if len(got[0]) != 2 || got[0][0] != 1 || got[1][0] != 2 {
		t.Fatalf("unexpected embeddings: %v", got)
	}
}

func TestOpenAIEmbedder_ObjectArrayResponse_NestedEmbedding_SingleInput(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode([]map[string]any{
			{"index": 0, "embedding": [][]float32{{0.7, 0.8}}},
		})
	}))
	defer srv.Close()

	cfg := newOpenAICfg("")
	cfg.OpenAIBaseURL = srv.URL
	e := NewOpenAIEmbedder(cfg, "text-embedding-3-small", cfg.OpenAIBaseURL)
	got, err := e.Embed(context.Background(), "hello")
	if err != nil {
		t.Fatalf("Embed: %v", err)
	}
	if len(got) != 2 || got[0] != 0.7 || got[1] != 0.8 {
		t.Fatalf("unexpected embedding: %v", got)
	}
}

func TestOpenAIEmbedder_EnvelopeResponse_NestedEmbedding_SingleInput(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"data": []map[string]any{
				{"index": 0, "embedding": [][]float32{{0.9, 1.1}}},
			},
		})
	}))
	defer srv.Close()

	cfg := newOpenAICfg("")
	cfg.OpenAIBaseURL = srv.URL
	e := NewOpenAIEmbedder(cfg, "text-embedding-3-small", cfg.OpenAIBaseURL)
	got, err := e.Embed(context.Background(), "hello")
	if err != nil {
		t.Fatalf("Embed: %v", err)
	}
	if len(got) != 2 || got[0] != 0.9 || got[1] != 1.1 {
		t.Fatalf("unexpected embedding: %v", got)
	}
}

func TestOpenAIEmbedder_ObjectArrayResponse_ColumnVector_SingleInput(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode([]map[string]any{
			{
				"index": 0,
				"embedding": [][]float32{
					{0.3},
					{0.4},
					{0.5},
				},
			},
		})
	}))
	defer srv.Close()

	cfg := newOpenAICfg("")
	cfg.OpenAIBaseURL = srv.URL
	e := NewOpenAIEmbedder(cfg, "text-embedding-3-small", cfg.OpenAIBaseURL)
	got, err := e.Embed(context.Background(), "hello")
	if err != nil {
		t.Fatalf("Embed: %v", err)
	}
	if len(got) != 3 || got[0] != 0.3 || got[1] != 0.4 || got[2] != 0.5 {
		t.Fatalf("unexpected embedding: %v", got)
	}
}

func TestOpenAIEmbedder_ObjectArrayResponse_MatrixEmbedding_SingleInput(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode([]map[string]any{
			{
				"index": 0,
				"embedding": [][]float32{
					{1, 10},
					{3, 30},
					{5, 50},
				},
			},
		})
	}))
	defer srv.Close()

	cfg := newOpenAICfg("")
	cfg.OpenAIBaseURL = srv.URL
	e := NewOpenAIEmbedder(cfg, "text-embedding-3-small", cfg.OpenAIBaseURL)
	got, err := e.Embed(context.Background(), "hello")
	if err != nil {
		t.Fatalf("Embed: %v", err)
	}
	// Mean-pool rows: [(1+3+5)/3, (10+30+50)/3]
	if len(got) != 2 || got[0] != 3 || got[1] != 30 {
		t.Fatalf("unexpected embedding: %v", got)
	}
}
