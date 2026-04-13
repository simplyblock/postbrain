package embedding

import (
	"context"
	"testing"

	"github.com/google/uuid"
)

// fakeEmbedderResolver is a test-only EmbedderResolver.
type fakeEmbedderResolver struct {
	embedder Embedder
}

func (f *fakeEmbedderResolver) EmbedderForModel(_ context.Context, _ uuid.UUID) (Embedder, error) {
	return f.embedder, nil
}

func TestEmbeddingService_EmbedTextResult_UsesActiveModelFactory(t *testing.T) {
	t.Parallel()

	modelID := uuid.New()
	svc := NewServiceFromEmbedders(&mockEmbedder{vec: []float32{9, 9}}, nil)
	svc.SetModelFactory(
		&fakeEmbedderResolver{embedder: &mockEmbedder{vec: []float32{1, 2, 3}}},
		nil,
		&modelID,
		nil,
		nil,
	)

	res, err := svc.EmbedTextResult(context.Background(), "hello")
	if err != nil {
		t.Fatalf("EmbedTextResult: %v", err)
	}
	if res == nil {
		t.Fatal("EmbedTextResult returned nil result")
	}
	if res.ModelID != modelID {
		t.Fatalf("ModelID = %s, want %s", res.ModelID, modelID)
	}
	if len(res.Embedding) != 3 || res.Embedding[0] != 1 {
		t.Fatalf("Embedding = %#v, want [1 2 3]", res.Embedding)
	}
}

func TestEmbeddingService_EmbedCodeResult_UsesActiveModelFactory(t *testing.T) {
	t.Parallel()

	modelID := uuid.New()
	svc := NewServiceFromEmbedders(&mockEmbedder{vec: []float32{9, 9}}, &mockEmbedder{vec: []float32{8, 8}})
	svc.SetModelFactory(
		&fakeEmbedderResolver{embedder: &mockEmbedder{vec: []float32{4, 5}}},
		nil,
		nil,
		&modelID,
		nil,
	)

	res, err := svc.EmbedCodeResult(context.Background(), "hello")
	if err != nil {
		t.Fatalf("EmbedCodeResult: %v", err)
	}
	if res == nil {
		t.Fatal("EmbedCodeResult returned nil result")
	}
	if res.ModelID != modelID {
		t.Fatalf("ModelID = %s, want %s", res.ModelID, modelID)
	}
	if len(res.Embedding) != 2 || res.Embedding[0] != 4 {
		t.Fatalf("Embedding = %#v, want [4 5]", res.Embedding)
	}
}
