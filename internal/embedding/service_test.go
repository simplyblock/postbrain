package embedding

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/simplyblock/postbrain/internal/config"
)

// mockEmbedder is a test-only Embedder implementation.
type mockEmbedder struct {
	slug string
	dims int
	vec  []float32
	err  error
}

func (m *mockEmbedder) Embed(_ context.Context, _ string) ([]float32, error) {
	return m.vec, m.err
}

func (m *mockEmbedder) EmbedBatch(_ context.Context, texts []string) ([][]float32, error) {
	if m.err != nil {
		return nil, m.err
	}
	out := make([][]float32, len(texts))
	for i := range texts {
		out[i] = m.vec
	}
	return out, nil
}

func (m *mockEmbedder) ModelSlug() string { return m.slug }
func (m *mockEmbedder) Dimensions() int   { return m.dims }

func ollamaCfgForService(textModel, codeModel string) *config.EmbeddingConfig {
	return &config.EmbeddingConfig{
		Backend:        "ollama",
		ServiceURL:     "http://localhost:11434",
		TextModel:      textModel,
		CodeModel:      codeModel,
		RequestTimeout: 5 * time.Second,
		BatchSize:      64,
	}
}

func openAICfgForService(textModel, codeModel string) *config.EmbeddingConfig {
	return &config.EmbeddingConfig{
		Backend:        "openai",
		OpenAIAPIKey:   "sk-test",
		TextModel:      textModel,
		CodeModel:      codeModel,
		RequestTimeout: 5 * time.Second,
		BatchSize:      64,
	}
}

func openAICfgForServiceWithBaseURL(textModel, codeModel, apiKey, baseURL string) *config.EmbeddingConfig {
	return &config.EmbeddingConfig{
		Backend:        "openai",
		OpenAIAPIKey:   apiKey,
		ServiceURL:     baseURL,
		TextModel:      textModel,
		CodeModel:      codeModel,
		RequestTimeout: 5 * time.Second,
		BatchSize:      64,
	}
}

// --- NewService construction tests ---

func TestNewService_OllamaBackendNoCodeModel(t *testing.T) {
	cfg := ollamaCfgForService("nomic-embed-text", "")
	svc, err := NewService(cfg)
	if err != nil {
		t.Fatalf("NewService: %v", err)
	}
	if svc.TextEmbedder() == nil {
		t.Error("TextEmbedder() should not be nil")
	}
	if svc.CodeEmbedder() != nil {
		t.Error("CodeEmbedder() should be nil when no code model configured")
	}
}

func TestNewService_OllamaBackendWithCodeModel(t *testing.T) {
	cfg := ollamaCfgForService("nomic-embed-text", "nomic-embed-code")
	svc, err := NewService(cfg)
	if err != nil {
		t.Fatalf("NewService: %v", err)
	}
	if svc.TextEmbedder() == nil {
		t.Error("TextEmbedder() should not be nil")
	}
	if svc.CodeEmbedder() == nil {
		t.Error("CodeEmbedder() should not be nil when code model configured")
	}
}

func TestNewService_OpenAIBackend(t *testing.T) {
	cfg := openAICfgForService("text-embedding-3-small", "")
	svc, err := NewService(cfg)
	if err != nil {
		t.Fatalf("NewService: %v", err)
	}
	if svc.TextEmbedder() == nil {
		t.Error("TextEmbedder() should not be nil")
	}
}

func TestNewService_OpenAIBackend_DefaultBaseURLRequiresAPIKey(t *testing.T) {
	cfg := openAICfgForServiceWithBaseURL("text-embedding-3-small", "", "", "")
	_, err := NewService(cfg)
	if err == nil {
		t.Fatal("expected error for missing openai_api_key with default base URL, got nil")
	}
	if !containsString(err.Error(), "openai_api_key") {
		t.Fatalf("error = %q, want mention of openai_api_key", err.Error())
	}
}

func TestNewService_OpenAIBackend_CustomBaseURLAllowsEmptyAPIKey(t *testing.T) {
	cfg := openAICfgForServiceWithBaseURL("text-embedding-3-small", "", "", "http://localhost:8080/v1")
	svc, err := NewService(cfg)
	if err != nil {
		t.Fatalf("NewService: %v", err)
	}
	text, ok := svc.TextEmbedder().(*OpenAIEmbedder)
	if !ok {
		t.Fatalf("TextEmbedder type = %T, want *OpenAIEmbedder", svc.TextEmbedder())
	}
	if text.baseURL != "http://localhost:8080/v1" {
		t.Fatalf("OpenAI baseURL = %q, want custom URL", text.baseURL)
	}
}

func TestNewService_UnsupportedBackendReturnsError(t *testing.T) {
	cfg := &config.EmbeddingConfig{
		Backend:   "cohere",
		TextModel: "embed-english-v3.0",
	}
	_, err := NewService(cfg)
	if err == nil {
		t.Fatal("expected error for unsupported backend, got nil")
	}
	if !containsString(err.Error(), "unsupported embedding backend") {
		t.Errorf("error message %q does not contain 'unsupported embedding backend'", err.Error())
	}
}

// --- EmbedText / EmbedCode tests using injected mock embedders ---

func newServiceWithMocks(textEmb, codeEmb Embedder) *EmbeddingService {
	return &EmbeddingService{text: textEmb, code: codeEmb}
}

func TestEmbedText_DelegatesToTextEmbedder(t *testing.T) {
	want := []float32{1.0, 2.0}
	svc := newServiceWithMocks(
		&mockEmbedder{slug: "text-model", vec: want},
		nil,
	)
	got, err := svc.EmbedText(context.Background(), "hello")
	if err != nil {
		t.Fatalf("EmbedText: %v", err)
	}
	for i, v := range want {
		if got[i] != v {
			t.Errorf("got[%d] = %v; want %v", i, got[i], v)
		}
	}
}

func TestEmbedCode_DelegatesToCodeEmbedder(t *testing.T) {
	textVec := []float32{1.0}
	codeVec := []float32{2.0, 3.0}
	svc := newServiceWithMocks(
		&mockEmbedder{slug: "text-model", vec: textVec},
		&mockEmbedder{slug: "code-model", vec: codeVec},
	)
	got, err := svc.EmbedCode(context.Background(), "func main() {}")
	if err != nil {
		t.Fatalf("EmbedCode: %v", err)
	}
	if got[0] != codeVec[0] {
		t.Errorf("got[0] = %v; want %v", got[0], codeVec[0])
	}
}

func TestEmbedCode_FallsBackToTextWhenNoCodeModel(t *testing.T) {
	textVec := []float32{9.0}
	svc := newServiceWithMocks(
		&mockEmbedder{slug: "text-model", vec: textVec},
		nil,
	)
	got, err := svc.EmbedCode(context.Background(), "func main() {}")
	if err != nil {
		t.Fatalf("EmbedCode fallback: %v", err)
	}
	if got[0] != textVec[0] {
		t.Errorf("got[0] = %v; want %v (fallback to text)", got[0], textVec[0])
	}
}

func TestEmbedText_PropagatesError(t *testing.T) {
	wantErr := errors.New("embed failed")
	svc := newServiceWithMocks(
		&mockEmbedder{slug: "text-model", err: wantErr},
		nil,
	)
	_, err := svc.EmbedText(context.Background(), "hello")
	if !errors.Is(err, wantErr) {
		t.Errorf("expected wrapped error %v, got %v", wantErr, err)
	}
}

// containsString reports whether s contains substr.
func containsString(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(substr) == 0 ||
		func() bool {
			for i := 0; i <= len(s)-len(substr); i++ {
				if s[i:i+len(substr)] == substr {
					return true
				}
			}
			return false
		}())
}
