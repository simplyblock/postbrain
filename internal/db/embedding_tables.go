package db

import (
	"context"
	"fmt"
	"strings"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

// EmbeddingTableName returns the per-model embedding table name.
// Naming is fixed to: embeddings_model_<uuid_no_dashes>.
func EmbeddingTableName(modelID uuid.UUID) string {
	return "embeddings_model_" + strings.ReplaceAll(modelID.String(), "-", "")
}

func embeddingHNSWIndexName(modelID uuid.UUID) string {
	id := strings.ReplaceAll(modelID.String(), "-", "")
	return "embm_" + id[:16] + "_hnsw"
}

func embeddingScopeIndexName(modelID uuid.UUID) string {
	id := strings.ReplaceAll(modelID.String(), "-", "")
	return "embm_" + id[:16] + "_scope_idx"
}

// EnsureEmbeddingModelTable creates (if needed) the per-model embedding table and indexes.
// The table stores one embedding vector per (object_type, object_id).
func EnsureEmbeddingModelTable(ctx context.Context, pool *pgxpool.Pool, modelID uuid.UUID, dims int) (string, error) {
	if pool == nil {
		return "", fmt.Errorf("db: ensure embedding model table: nil pool")
	}
	if dims <= 0 {
		return "", fmt.Errorf("db: ensure embedding model table: invalid dimensions %d", dims)
	}

	tableName := EmbeddingTableName(modelID)
	hnswIndex := embeddingHNSWIndexName(modelID)
	scopeIndex := embeddingScopeIndexName(modelID)

	createTableSQL := fmt.Sprintf(`
CREATE TABLE IF NOT EXISTS %s (
    object_type  TEXT        NOT NULL,
    object_id    UUID        NOT NULL,
    scope_id     UUID        NOT NULL REFERENCES scopes(id) ON DELETE CASCADE,
    embedding    vector(%d)  NOT NULL,
    created_at   TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at   TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (object_type, object_id)
);`, tableName, dims)
	if _, err := pool.Exec(ctx, createTableSQL); err != nil {
		return "", fmt.Errorf("db: ensure embedding model table create: %w", err)
	}

	createScopeIndexSQL := fmt.Sprintf(`CREATE INDEX IF NOT EXISTS %s ON %s (scope_id);`, scopeIndex, tableName)
	if _, err := pool.Exec(ctx, createScopeIndexSQL); err != nil {
		return "", fmt.Errorf("db: ensure embedding model table scope index: %w", err)
	}

	createHNSWIndexSQL := fmt.Sprintf(`CREATE INDEX IF NOT EXISTS %s ON %s USING hnsw (embedding vector_cosine_ops);`, hnswIndex, tableName)
	if _, err := pool.Exec(ctx, createHNSWIndexSQL); err != nil {
		return "", fmt.Errorf("db: ensure embedding model table hnsw index: %w", err)
	}

	createTriggerSQL := fmt.Sprintf(`
DO $$
BEGIN
  IF NOT EXISTS (
    SELECT 1 FROM pg_trigger
    WHERE tgname = %s
      AND tgrelid = %s::regclass
  ) THEN
    EXECUTE %s;
  END IF;
END$$;`,
		quoteLiteral(tableName+"_updated_at"),
		quoteLiteral(tableName),
		quoteLiteral(fmt.Sprintf("CREATE TRIGGER %s BEFORE UPDATE ON %s FOR EACH ROW EXECUTE FUNCTION touch_updated_at()",
			tableName+"_updated_at", tableName)),
	)
	if _, err := pool.Exec(ctx, createTriggerSQL); err != nil {
		return "", fmt.Errorf("db: ensure embedding model table trigger: %w", err)
	}

	return tableName, nil
}

func quoteLiteral(s string) string {
	return "'" + strings.ReplaceAll(s, "'", "''") + "'"
}
