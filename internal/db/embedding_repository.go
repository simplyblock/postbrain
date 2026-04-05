package db

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// ScopeFilter defines the scope boundary applied to embedding queries.
type ScopeFilter struct {
	ScopePath  string
	AgentType  string
	Visibility []string
}

// EmbeddingQuery defines a model-scoped ANN query.
type EmbeddingQuery struct {
	ModelID    uuid.UUID
	ObjectType string
	Embedding  []float32
	Limit      int
	Scope      *ScopeFilter
}

// EmbeddingHit is one ANN query result row.
type EmbeddingHit struct {
	ObjectID uuid.UUID
	Score    float64
}

// UpsertEmbeddingInput is the write contract for one embedding row.
type UpsertEmbeddingInput struct {
	ObjectType string
	ObjectID   uuid.UUID
	ScopeID    uuid.UUID
	ModelID    uuid.UUID
	Embedding  []float32
}

// EmbeddingRepository provides model-table-backed embedding storage.
type EmbeddingRepository struct {
	pool *pgxpool.Pool
}

// NewEmbeddingRepository creates a repository instance for embedding tables.
func NewEmbeddingRepository(pool *pgxpool.Pool) *EmbeddingRepository {
	return &EmbeddingRepository{pool: pool}
}

// UpsertEmbedding writes one embedding and marks embedding_index as ready.
func (r *EmbeddingRepository) UpsertEmbedding(ctx context.Context, in UpsertEmbeddingInput) error {
	if r == nil || r.pool == nil {
		return fmt.Errorf("embedding repository: nil pool")
	}
	if err := validateObjectType(in.ObjectType); err != nil {
		return err
	}
	if len(in.Embedding) == 0 {
		return fmt.Errorf("embedding repository: embedding is empty")
	}

	return runWithRetry(ctx, defaultRetryAttempts, func() error {
		tx, err := r.pool.Begin(ctx)
		if err != nil {
			return fmt.Errorf("embedding repository: begin tx: %w", err)
		}
		defer tx.Rollback(ctx) //nolint:errcheck

		meta, err := lookupReadyModelTableMetadata(ctx, tx, in.ModelID)
		if err != nil {
			return err
		}
		if len(in.Embedding) != meta.dimensions {
			return fmt.Errorf("embedding repository: dimension mismatch: have %d want %d", len(in.Embedding), meta.dimensions)
		}

		sql := fmt.Sprintf(`
			INSERT INTO %s (object_type, object_id, scope_id, embedding)
			VALUES ($1, $2, $3, $4::vector)
			ON CONFLICT (object_type, object_id)
			DO UPDATE SET scope_id = EXCLUDED.scope_id, embedding = EXCLUDED.embedding, updated_at = now()
		`, meta.tableName)
		if _, err := tx.Exec(ctx, sql, in.ObjectType, in.ObjectID, in.ScopeID, ExportFloat32SliceToVector(in.Embedding)); err != nil {
			return fmt.Errorf("embedding repository: upsert embedding: %w", err)
		}

		if _, err := tx.Exec(ctx, `
			INSERT INTO embedding_index (object_type, object_id, model_id, status, retry_count, last_error)
			VALUES ($1, $2, $3, 'ready', 0, NULL)
			ON CONFLICT (object_type, object_id, model_id)
			DO UPDATE SET status = 'ready', retry_count = 0, last_error = NULL, updated_at = now()
		`, in.ObjectType, in.ObjectID, in.ModelID); err != nil {
			return fmt.Errorf("embedding repository: upsert embedding index: %w", err)
		}

		if err := tx.Commit(ctx); err != nil {
			return fmt.Errorf("embedding repository: commit tx: %w", err)
		}
		return nil
	})
}

// GetEmbedding loads one embedding vector from the model table.
func (r *EmbeddingRepository) GetEmbedding(ctx context.Context, modelID uuid.UUID, objectType string, objectID uuid.UUID) ([]float32, error) {
	if r == nil || r.pool == nil {
		return nil, fmt.Errorf("embedding repository: nil pool")
	}
	if err := validateObjectType(objectType); err != nil {
		return nil, err
	}

	meta, err := lookupReadyModelTableMetadata(ctx, r.pool, modelID)
	if err != nil {
		return nil, err
	}

	sql := fmt.Sprintf(`
		SELECT embedding::text
		FROM %s
		WHERE object_type = $1 AND object_id = $2
	`, meta.tableName)
	var literal string
	err = r.pool.QueryRow(ctx, sql, objectType, objectID).Scan(&literal)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil
		}
		return nil, fmt.Errorf("embedding repository: get embedding: %w", err)
	}
	vec, err := parseVectorLiteral(literal)
	if err != nil {
		return nil, fmt.Errorf("embedding repository: parse vector literal: %w", err)
	}
	return vec, nil
}

// QuerySimilar executes an ANN similarity query against one model table.
func (r *EmbeddingRepository) QuerySimilar(ctx context.Context, q EmbeddingQuery) ([]EmbeddingHit, error) {
	if r == nil || r.pool == nil {
		return nil, fmt.Errorf("embedding repository: nil pool")
	}
	if err := validateObjectType(q.ObjectType); err != nil {
		return nil, err
	}
	if len(q.Embedding) == 0 {
		return nil, fmt.Errorf("embedding repository: query embedding is empty")
	}
	limit := q.Limit
	if limit <= 0 {
		limit = 20
	}

	meta, err := lookupReadyModelTableMetadata(ctx, r.pool, q.ModelID)
	if err != nil {
		return nil, err
	}
	if len(q.Embedding) != meta.dimensions {
		return nil, fmt.Errorf("embedding repository: dimension mismatch: have %d want %d", len(q.Embedding), meta.dimensions)
	}
	distanceExpr := similarityDistanceExpr(meta.dimensions)

	join, baseWhere := objectTypeJoinAndWhere(q.ObjectType)
	conds := "t.object_type = $2" + baseWhere
	args := []any{ExportFloat32SliceToVector(q.Embedding), q.ObjectType, limit}
	if q.Scope != nil && strings.TrimSpace(q.Scope.ScopePath) != "" {
		conds += " AND sc.path <@ $4::ltree"
		args = append(args, q.Scope.ScopePath)
	}

	sql := fmt.Sprintf(`
		SELECT t.object_id, 1 - (%s) AS score
		FROM %s t
		%s
		WHERE %s
		ORDER BY %s
		LIMIT $3
	`, distanceExpr, meta.tableName, join, conds, distanceExpr)
	rows, err := r.pool.Query(ctx, sql, args...)
	if err != nil {
		return nil, fmt.Errorf("embedding repository: query similar: %w", err)
	}
	defer rows.Close()

	hits := make([]EmbeddingHit, 0, limit)
	for rows.Next() {
		var h EmbeddingHit
		if err := rows.Scan(&h.ObjectID, &h.Score); err != nil {
			return nil, fmt.Errorf("embedding repository: query similar scan: %w", err)
		}
		hits = append(hits, h)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("embedding repository: query similar rows: %w", err)
	}
	return hits, nil
}

func validateObjectType(objectType string) error {
	switch objectType {
	case "memory", "entity", "knowledge_artifact", "skill":
		return nil
	default:
		return fmt.Errorf("embedding repository: invalid object type %q", objectType)
	}
}

func isSafeTableName(name string) bool {
	if !strings.HasPrefix(name, "embeddings_model_") {
		return false
	}
	for _, r := range name {
		if r != '_' && (r < 'a' || r > 'z') && (r < '0' || r > '9') {
			return false
		}
	}
	return true
}

func objectTypeJoinAndWhere(objectType string) (joinSQL, whereSQL string) {
	switch objectType {
	case "memory":
		return "JOIN memories obj ON obj.id = t.object_id JOIN scopes sc ON sc.id = obj.scope_id", " AND obj.is_active = true"
	case "entity":
		return "JOIN entities obj ON obj.id = t.object_id JOIN scopes sc ON sc.id = obj.scope_id", ""
	case "knowledge_artifact":
		return "JOIN knowledge_artifacts obj ON obj.id = t.object_id JOIN scopes sc ON sc.id = obj.owner_scope_id", ""
	case "skill":
		return "JOIN skills obj ON obj.id = t.object_id JOIN scopes sc ON sc.id = obj.scope_id", ""
	default:
		return "", ""
	}
}

func parseVectorLiteral(lit string) ([]float32, error) {
	trimmed := strings.TrimSpace(lit)
	if len(trimmed) < 2 || trimmed[0] != '[' || trimmed[len(trimmed)-1] != ']' {
		return nil, fmt.Errorf("invalid vector literal %q", lit)
	}
	body := strings.TrimSpace(trimmed[1 : len(trimmed)-1])
	if body == "" {
		return []float32{}, nil
	}
	parts := strings.Split(body, ",")
	out := make([]float32, 0, len(parts))
	for _, p := range parts {
		v, err := strconv.ParseFloat(strings.TrimSpace(p), 32)
		if err != nil {
			return nil, err
		}
		out = append(out, float32(v))
	}
	return out, nil
}

func similarityDistanceExpr(dims int) string {
	if dims > maxVectorHNSWDimensions {
		return fmt.Sprintf("t.embedding::halfvec(%d) <=> $1::halfvec", dims)
	}
	return "t.embedding <=> $1::vector"
}
