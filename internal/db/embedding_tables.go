package db

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

const maxVectorHNSWDimensions = 2000

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
	return ensureEmbeddingModelTable(ctx, pool, modelID, dims)
}

func ensureEmbeddingModelTable(ctx context.Context, execer DBTX, modelID uuid.UUID, dims int) (string, error) {
	if dims <= 0 {
		return "", fmt.Errorf("db: ensure embedding model table: invalid dimensions %d", dims)
	}

	tableName := EmbeddingTableName(modelID)
	hnswIndex := embeddingHNSWIndexName(modelID)
	scopeIndex := embeddingScopeIndexName(modelID)
	columnType, indexExpr, opClass := embeddingStorageForDimensions(dims)

	existingType, existingDims, exists, err := readEmbeddingTableDimensions(ctx, execer, tableName)
	if err != nil {
		return "", fmt.Errorf("db: ensure embedding model table inspect: %w", err)
	}
	if exists && existingDims != dims {
		return "", fmt.Errorf(
			"db: ensure embedding model table: dimension mismatch for %s: have %d want %d",
			tableName, existingDims, dims,
		)
	}
	// The table always stores full-precision vector(dims). High-dimension models
	// use an HNSW expression index on embedding::halfvec(dims).
	if exists && existingType != columnType {
		return "", fmt.Errorf("db: ensure embedding model table: type mismatch for %s: have %s want %s", tableName, existingType, columnType)
	}

	createTableSQL := fmt.Sprintf(`
CREATE TABLE IF NOT EXISTS %s (
    object_type  TEXT        NOT NULL,
    object_id    UUID        NOT NULL,
    scope_id     UUID        NOT NULL REFERENCES scopes(id) ON DELETE CASCADE,
    embedding    %s          NOT NULL,
    created_at   TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at   TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (object_type, object_id)
);`, tableName, columnType)
	if _, err := execer.Exec(ctx, createTableSQL); err != nil {
		return "", fmt.Errorf("db: ensure embedding model table create: %w", err)
	}

	createScopeIndexSQL := fmt.Sprintf(`CREATE INDEX IF NOT EXISTS %s ON %s (scope_id);`, scopeIndex, tableName)
	if _, err := execer.Exec(ctx, createScopeIndexSQL); err != nil {
		return "", fmt.Errorf("db: ensure embedding model table scope index: %w", err)
	}

	createHNSWIndexSQL := fmt.Sprintf(`CREATE INDEX IF NOT EXISTS %s ON %s USING hnsw (%s %s);`, hnswIndex, tableName, indexExpr, opClass)
	if _, err := execer.Exec(ctx, createHNSWIndexSQL); err != nil {
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
	if _, err := execer.Exec(ctx, createTriggerSQL); err != nil {
		return "", fmt.Errorf("db: ensure embedding model table trigger: %w", err)
	}

	return tableName, nil
}

func readEmbeddingTableDimensions(ctx context.Context, execer DBTX, tableName string) (string, int, bool, error) {
	var typ *string
	err := execer.QueryRow(ctx, `
		SELECT format_type(a.atttypid, a.atttypmod)
		FROM pg_attribute a
		JOIN pg_class c ON c.oid = a.attrelid
		WHERE c.relname = $1 AND a.attname = 'embedding' AND a.attnum > 0 AND NOT a.attisdropped
	`, tableName).Scan(&typ)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return "", 0, false, nil
		}
		return "", 0, false, err
	}
	if typ == nil || *typ == "" {
		return "", 0, false, nil
	}

	typeName := strings.TrimSpace(*typ)
	var dims int
	if _, scanErr := fmt.Sscanf(typeName, "vector(%d)", &dims); scanErr == nil {
		return typeName, dims, true, nil
	}
	if _, scanErr := fmt.Sscanf(typeName, "halfvec(%d)", &dims); scanErr == nil {
		return typeName, dims, true, nil
	}
	return "", 0, false, fmt.Errorf("parse embedding type %q: unsupported embedding column type", *typ)
}

func quoteLiteral(s string) string {
	return "'" + strings.ReplaceAll(s, "'", "''") + "'"
}

func embeddingStorageForDimensions(dims int) (columnType string, indexExpr string, opClass string) {
	if dims > maxVectorHNSWDimensions {
		return fmt.Sprintf("vector(%d)", dims), fmt.Sprintf("(embedding::halfvec(%d))", dims), "halfvec_cosine_ops"
	}
	return fmt.Sprintf("vector(%d)", dims), "embedding", "vector_cosine_ops"
}
