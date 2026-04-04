package db

import (
	"strings"
	"testing"

	"github.com/google/uuid"
)

func TestEmbeddingTableName_UsesUUIDWithoutDashes(t *testing.T) {
	modelID := uuid.MustParse("123e4567-e89b-12d3-a456-426614174000")
	got := EmbeddingTableName(modelID)
	want := "embeddings_model_123e4567e89b12d3a456426614174000"
	if got != want {
		t.Fatalf("EmbeddingTableName() = %q, want %q", got, want)
	}
	if strings.Contains(got, "-") {
		t.Fatalf("EmbeddingTableName() contains dashes: %q", got)
	}
}

func TestEmbeddingTableName_UsesExpectedPrefix(t *testing.T) {
	got := EmbeddingTableName(uuid.New())
	if !strings.HasPrefix(got, "embeddings_model_") {
		t.Fatalf("EmbeddingTableName() = %q, want prefix embeddings_model_", got)
	}
}
