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

func TestEmbeddingStorageForDimensions_Vector(t *testing.T) {
	columnType, indexExpr, opClass := embeddingStorageForDimensions(1536)
	if columnType != "vector(1536)" {
		t.Fatalf("columnType = %q, want vector(1536)", columnType)
	}
	if indexExpr != "embedding" {
		t.Fatalf("indexExpr = %q, want embedding", indexExpr)
	}
	if opClass != "vector_cosine_ops" {
		t.Fatalf("opClass = %q, want vector_cosine_ops", opClass)
	}
}

func TestEmbeddingStorageForDimensions_Halfvec(t *testing.T) {
	columnType, indexExpr, opClass := embeddingStorageForDimensions(2560)
	if columnType != "vector(2560)" {
		t.Fatalf("columnType = %q, want vector(2560)", columnType)
	}
	if indexExpr != "(embedding::halfvec(2560))" {
		t.Fatalf("indexExpr = %q, want (embedding::halfvec(2560))", indexExpr)
	}
	if opClass != "halfvec_cosine_ops" {
		t.Fatalf("opClass = %q, want halfvec_cosine_ops", opClass)
	}
}
