package jobs

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	pgvector "github.com/pgvector/pgvector-go"

	"github.com/simplyblock/postbrain/internal/db"
	"github.com/simplyblock/postbrain/internal/embedding"
	"github.com/simplyblock/postbrain/internal/retrieval"
)

const (
	contradictionBatchSize      = 100
	topicSimilarityThreshold    = 0.6
	negationSimilarityThreshold = 0.5
)

// ContradictionJob runs the weekly contradiction detection (Signal 2).
// It compares published knowledge artifacts against recent memories and flags
// artifacts that appear to be contradicted by recent observations.
type ContradictionJob struct {
	pool *pgxpool.Pool
	svc  *embedding.EmbeddingService
	// classify is injected to allow testing without a real LLM.
	// It returns one of: "CONTRADICTS", "CONSISTENT", "UNRELATED"
	classify func(ctx context.Context, artifactContent, memoryContent string) (verdict, reasoning string, err error)
}

// NewContradictionJob creates a new ContradictionJob. If classify is nil, a
// no-op classifier that always returns "CONSISTENT" is used (safe default for
// deployments without LLM).
func NewContradictionJob(pool *pgxpool.Pool, svc *embedding.EmbeddingService, classify func(ctx context.Context, artifact, memory string) (string, string, error)) *ContradictionJob {
	if classify == nil {
		classify = noopClassifier
	}
	return &ContradictionJob{
		pool:     pool,
		svc:      svc,
		classify: classify,
	}
}

// Run executes the full contradiction detection pipeline.
func (j *ContradictionJob) Run(ctx context.Context) error {
	_, err := RunPaginatedBatch(ctx, contradictionBatchSize,
		func(ctx context.Context, limit, offset int) ([]*db.KnowledgeArtifact, error) {
			return j.fetchArtifactBatch(ctx, offset)
		},
		func(ctx context.Context, artifact *db.KnowledgeArtifact) {
			if err := j.processArtifact(ctx, artifact); err != nil {
				slog.Error("contradiction: process artifact failed",
					"artifact_id", artifact.ID, "error", err)
			}
		},
	)
	if err != nil {
		return fmt.Errorf("contradiction: %w", err)
	}
	return nil
}

// fetchArtifactBatch fetches a batch of published knowledge artifacts.
func (j *ContradictionJob) fetchArtifactBatch(ctx context.Context, offset int) ([]*db.KnowledgeArtifact, error) {
	rows, err := j.pool.Query(ctx,
		`SELECT id, knowledge_type, owner_scope_id, author_id,
		        visibility, status, published_at, deprecated_at, review_required,
		        title, content, summary, embedding::text, embedding_model_id, meta,
		        endorsement_count, access_count, last_accessed,
		        version, previous_version, source_memory_id, source_ref,
		        created_at, updated_at
		 FROM knowledge_artifacts
		 WHERE status='published'
		 ORDER BY created_at
		 LIMIT $1 OFFSET $2`,
		contradictionBatchSize, offset,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var artifacts []*db.KnowledgeArtifact
	for rows.Next() {
		var a db.KnowledgeArtifact
		var embText *string
		var summary *string
		err := rows.Scan(
			&a.ID, &a.KnowledgeType, &a.OwnerScopeID, &a.AuthorID,
			&a.Visibility, &a.Status, &a.PublishedAt, &a.DeprecatedAt, &a.ReviewRequired,
			&a.Title, &a.Content, &summary, &embText, &a.EmbeddingModelID, &a.Meta,
			&a.EndorsementCount, &a.AccessCount, &a.LastAccessed,
			&a.Version, &a.PreviousVersion, &a.SourceMemoryID, &a.SourceRef,
			&a.CreatedAt, &a.UpdatedAt,
		)
		if err != nil {
			return nil, err
		}
		a.Summary = summary
		a.Embedding = scanPgVector(embText)
		artifacts = append(artifacts, &a)
	}
	return artifacts, rows.Err()
}

// processArtifact runs contradiction detection for a single artifact.
func (j *ContradictionJob) processArtifact(ctx context.Context, artifact *db.KnowledgeArtifact) error {
	// Fetch recent memories from same/ancestor scopes (last 7 days).
	memories, err := j.fetchRecentMemories(ctx, artifact.OwnerScopeID)
	if err != nil {
		return fmt.Errorf("fetch recent memories: %w", err)
	}
	if len(memories) == 0 {
		return nil
	}

	// Pre-filter by topic overlap: cosine similarity > 0.6.
	// Skip artifacts with no embedding — without a vector we can't compare topics.
	if artifact.Embedding == nil {
		return nil
	}
	topicMatches := j.filterByTopicSimilarity(artifact.Embedding.Slice(), memories)
	if len(topicMatches) == 0 {
		return nil
	}

	// Negation pre-filter: embed "artifact.Title is false, wrong, or outdated"
	// and keep only memories with similarity > 0.5.
	negationQuery := artifact.Title + " is false, wrong, or outdated"
	negationVec, err := j.svc.EmbedText(ctx, negationQuery)
	if err != nil {
		return fmt.Errorf("embed negation query: %w", err)
	}

	type survivor struct {
		mem         *db.Memory
		negSimScore float64
	}
	var survivors []survivor
	for _, m := range topicMatches {
		sim := retrieval.CosineSimilarity(negationVec, m.Embedding.Slice())
		if sim > negationSimilarityThreshold {
			survivors = append(survivors, survivor{mem: m, negSimScore: sim})
		}
	}
	if len(survivors) == 0 {
		return nil
	}

	// Check if there is already an open contradiction flag.
	hasFlag, err := db.HasOpenStalenessFlag(ctx, j.pool, artifact.ID, "contradiction_detected")
	if err != nil {
		return fmt.Errorf("check open flag: %w", err)
	}
	if hasFlag {
		return nil
	}

	// For each survivor, run the LLM classifier.
	for _, s := range survivors {
		verdict, reasoning, err := j.classify(ctx, artifact.Content, s.mem.Content)
		if err != nil {
			slog.Error("contradiction: classifier error",
				"artifact_id", artifact.ID, "memory_id", s.mem.ID, "error", err)
			continue
		}
		if verdict != "CONTRADICTS" {
			continue
		}

		// confidence = min(0.9, negationSimilarity * 1.5)
		confidence := s.negSimScore * 1.5
		if confidence > 0.9 {
			confidence = 0.9
		}

		memIDs := []string{s.mem.ID.String()}
		evidence, _ := json.Marshal(map[string]any{
			"memory_ids":           memIDs,
			"classifier_verdict":   "CONTRADICTS",
			"classifier_reasoning": reasoning,
		})

		flag := &db.StalenessFlag{
			ArtifactID: artifact.ID,
			Signal:     "contradiction_detected",
			Confidence: confidence,
			Evidence:   evidence,
		}
		if _, err := db.InsertStalenessFlag(ctx, j.pool, flag); err != nil {
			slog.Error("contradiction: insert staleness flag failed",
				"artifact_id", artifact.ID, "error", err)
		} else {
			slog.Info("contradiction: flagged artifact",
				"artifact_id", artifact.ID, "memory_id", s.mem.ID, "confidence", confidence)
		}
		// Only insert one flag per artifact per run.
		break
	}
	return nil
}

// fetchRecentMemories fetches active memories from the last 7 days in the
// same scope or ancestor scopes.
func (j *ContradictionJob) fetchRecentMemories(ctx context.Context, scopeID uuid.UUID) ([]*db.Memory, error) {
	rows, err := j.pool.Query(ctx,
		`SELECT m.id, m.memory_type, m.scope_id, m.author_id,
		        m.content, m.summary, m.embedding::text, m.embedding_model_id,
		        m.embedding_code::text, m.embedding_code_model_id, m.content_kind, m.meta,
		        m.version, m.is_active, m.confidence, m.importance, m.access_count,
		        m.last_accessed, m.expires_at, m.promotion_status, m.promoted_to,
		        m.source_ref, m.created_at, m.updated_at
		 FROM memories m
		 JOIN scopes s ON m.scope_id = s.id
		 WHERE m.is_active=true
		   AND m.created_at > now() - INTERVAL '7 days'
		   AND s.path @> (SELECT path FROM scopes WHERE id = $1)`,
		scopeID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var memories []*db.Memory
	for rows.Next() {
		var m db.Memory
		var embText, embCodeText *string
		err := rows.Scan(
			&m.ID, &m.MemoryType, &m.ScopeID, &m.AuthorID,
			&m.Content, &m.Summary, &embText, &m.EmbeddingModelID,
			&embCodeText, &m.EmbeddingCodeModelID, &m.ContentKind, &m.Meta,
			&m.Version, &m.IsActive, &m.Confidence, &m.Importance, &m.AccessCount,
			&m.LastAccessed, &m.ExpiresAt, &m.PromotionStatus, &m.PromotedTo,
			&m.SourceRef, &m.CreatedAt, &m.UpdatedAt,
		)
		if err != nil {
			return nil, err
		}
		m.Embedding = scanPgVector(embText)
		m.EmbeddingCode = scanPgVector(embCodeText)
		memories = append(memories, &m)
	}
	return memories, rows.Err()
}

// scanPgVector parses a pgvector stored as text (via ::text cast) into a
// *pgvector.Vector. Returns nil if text is nil or the parse fails.
func scanPgVector(text *string) *pgvector.Vector {
	if text == nil {
		return nil
	}
	var v pgvector.Vector
	if err := v.Scan(*text); err != nil {
		return nil
	}
	return &v
}

// filterByTopicSimilarity returns memories whose text embedding has cosine
// similarity > topicSimilarityThreshold with the artifact embedding.
func (j *ContradictionJob) filterByTopicSimilarity(artifactEmbedding []float32, memories []*db.Memory) []*db.Memory {
	if len(artifactEmbedding) == 0 {
		return memories
	}
	var result []*db.Memory
	for _, m := range memories {
		if m.Embedding == nil || len(m.Embedding.Slice()) == 0 {
			continue
		}
		sim := retrieval.CosineSimilarity(artifactEmbedding, m.Embedding.Slice())
		if sim > topicSimilarityThreshold {
			result = append(result, m)
		}
	}
	return result
}

// noopClassifier is the default classifier for deployments without an LLM.
// It always returns "CONSISTENT" so no staleness flags are ever inserted.
func noopClassifier(_ context.Context, _, _ string) (string, string, error) {
	return "CONSISTENT", "no-op classifier: no LLM configured", nil
}
