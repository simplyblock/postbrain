package embedding

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/simplyblock/postbrain/internal/config"
)

func newOllamaCfg(url string) *config.EmbeddingConfig {
	return &config.EmbeddingConfig{
		Backend:        "ollama",
		ServiceURL:     url,
		TextModel:      "nomic-embed-text",
		RequestTimeout: 5 * time.Second,
		BatchSize:      64,
	}
}

func TestOllamaEmbedder_SuccessfulEmbed(t *testing.T) {
	want := []float32{0.1, 0.2, 0.3}

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("expected POST, got %s", r.Method)
		}
		if !strings.HasSuffix(r.URL.Path, "/api/embeddings") {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		var req map[string]interface{}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Errorf("decode body: %v", err)
		}
		w.Header().Set("Content-Type", "application/json")
		if err := json.NewEncoder(w).Encode(map[string]interface{}{
			"embedding": want,
		}); err != nil {
			t.Errorf("encode response: %v", err)
		}
	}))
	defer srv.Close()

	e := NewOllamaEmbedder(newOllamaCfg(srv.URL), "nomic-embed-text")
	got, err := e.Embed(context.Background(), "hello world")
	if err != nil {
		t.Fatalf("Embed: %v", err)
	}
	if len(got) != len(want) {
		t.Fatalf("len(got) = %d; want %d", len(got), len(want))
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("got[%d] = %v; want %v", i, got[i], want[i])
		}
	}
}

func TestOllamaEmbedder_EmptyEmbeddingReturnsError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if err := json.NewEncoder(w).Encode(map[string]interface{}{
			"embedding": []float32{},
		}); err != nil {
			t.Errorf("encode response: %v", err)
		}
	}))
	defer srv.Close()

	e := NewOllamaEmbedder(newOllamaCfg(srv.URL), "nomic-embed-text")
	_, err := e.Embed(context.Background(), "hello")
	if err == nil {
		t.Fatal("expected error for empty embedding, got nil")
	}
}

func TestOllamaEmbedder_HTTPErrorReturnsError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "internal server error", http.StatusInternalServerError)
	}))
	defer srv.Close()

	e := NewOllamaEmbedder(newOllamaCfg(srv.URL), "nomic-embed-text")
	_, err := e.Embed(context.Background(), "hello")
	if err == nil {
		t.Fatal("expected error for HTTP 500, got nil")
	}
}

func TestOllamaEmbedder_ContextCancellationReturnsError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Block until context is cancelled.
		<-r.Context().Done()
	}))
	defer srv.Close()

	e := NewOllamaEmbedder(newOllamaCfg(srv.URL), "nomic-embed-text")
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // cancel immediately

	_, err := e.Embed(ctx, "hello")
	if err == nil {
		t.Fatal("expected error for cancelled context, got nil")
	}
}

func TestOllamaEmbedder_ModelSlugAndDimensions(t *testing.T) {
	want := []float32{0.1, 0.2}

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if err := json.NewEncoder(w).Encode(map[string]interface{}{
			"embedding": want,
		}); err != nil {
			t.Errorf("encode response: %v", err)
		}
	}))
	defer srv.Close()

	e := NewOllamaEmbedder(newOllamaCfg(srv.URL), "nomic-embed-text")
	if e.ModelSlug() != "nomic-embed-text" {
		t.Errorf("ModelSlug() = %q; want %q", e.ModelSlug(), "nomic-embed-text")
	}
	// Before first call, Dimensions returns -1.
	if d := e.Dimensions(); d != -1 {
		t.Errorf("Dimensions() before first call = %d; want -1", d)
	}

	_, err := e.Embed(context.Background(), "hello")
	if err != nil {
		t.Fatalf("Embed: %v", err)
	}

	// After first call, Dimensions should be cached.
	if d := e.Dimensions(); d != len(want) {
		t.Errorf("Dimensions() after first call = %d; want %d", d, len(want))
	}
}
