package compat

import (
	"context"
	"fmt"
	"math"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/simplyblock/postbrain/internal/db"
)

// CreateArtifact inserts a new knowledge artifact.
func CreateArtifact(ctx context.Context, pool *pgxpool.Pool, a *db.KnowledgeArtifact) (*db.KnowledgeArtifact, error) {
	if a.Meta == nil {
		a.Meta = []byte("{}")
	}
	if a.ArtifactKind == "" {
		a.ArtifactKind = "general"
	}
	q := db.New(pool)
	result, err := q.CreateArtifact(ctx, db.CreateArtifactParams{
		KnowledgeType:    a.KnowledgeType,
		ArtifactKind:     a.ArtifactKind,
		OwnerScopeID:     a.OwnerScopeID,
		AuthorID:         a.AuthorID,
		Visibility:       a.Visibility,
		Status:           a.Status,
		PublishedAt:      a.PublishedAt,
		DeprecatedAt:     a.DeprecatedAt,
		ReviewRequired:   a.ReviewRequired,
		Title:            a.Title,
		Content:          a.Content,
		Summary:          a.Summary,
		Embedding:        a.Embedding,
		EmbeddingModelID: a.EmbeddingModelID,
		Meta:             a.Meta,
		Version:          a.Version,
		PreviousVersion:  nilIfZeroUUID(a.PreviousVersion),
		SourceMemoryID:   nilIfZeroUUID(a.SourceMemoryID),
		SourceRef:        a.SourceRef,
	})
	if err != nil {
		return nil, fmt.Errorf("db: create artifact: %w", err)
	}
	return knowledgeArtifactFromCreateArtifactRow(result), nil
}

// GetArtifact retrieves a knowledge artifact by ID. Returns nil, nil if not found.
func GetArtifact(ctx context.Context, pool *pgxpool.Pool, id uuid.UUID) (*db.KnowledgeArtifact, error) {
	q := db.New(pool)
	a, err := q.GetArtifact(ctx, id)
	if err == pgx.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("db: get artifact: %w", err)
	}
	return knowledgeArtifactFromGetArtifactRow(a), nil
}

// UpdateArtifact updates title, content, summary, embedding, and bumps version.
func UpdateArtifact(ctx context.Context, pool *pgxpool.Pool, id uuid.UUID, title, content string, summary *string, embedding []float32, modelID *uuid.UUID) (*db.KnowledgeArtifact, error) {
	q := db.New(pool)
	a, err := q.UpdateArtifact(ctx, db.UpdateArtifactParams{
		ID:               id,
		Title:            title,
		Content:          content,
		Summary:          summary,
		Embedding:        vecPtr(embedding),
		EmbeddingModelID: modelID,
	})
	if err == pgx.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("db: update artifact: %w", err)
	}
	return knowledgeArtifactFromUpdateArtifactRow(a), nil
}

// DeleteArtifact permanently removes a knowledge artifact by ID.
// Callers must pre-null any NO ACTION FK references before calling this.
func DeleteArtifact(ctx context.Context, pool *pgxpool.Pool, id uuid.UUID) error {
	q := db.New(pool)
	return q.DeleteArtifact(ctx, id)
}

// NullPreviousVersionRefs clears self-referential previous_version FK pointing at id.
func NullPreviousVersionRefs(ctx context.Context, pool *pgxpool.Pool, id uuid.UUID) error {
	q := db.New(pool)
	return q.NullPreviousVersionRefs(ctx, &id)
}

// NullPromotionRequestArtifactRef clears result_artifact_id FK in promotion_requests.
func NullPromotionRequestArtifactRef(ctx context.Context, pool *pgxpool.Pool, id uuid.UUID) error {
	q := db.New(pool)
	return q.NullPromotionRequestArtifactRef(ctx, &id)
}

// ResetPromotedMemoryStatus clears promotion_status on memories whose promoted_to points at id.
func ResetPromotedMemoryStatus(ctx context.Context, pool *pgxpool.Pool, id uuid.UUID) error {
	q := db.New(pool)
	return q.ResetPromotedMemoryStatus(ctx, &id)
}

// UpdateArtifactStatus updates status, published_at, and deprecated_at.
func UpdateArtifactStatus(ctx context.Context, pool *pgxpool.Pool, id uuid.UUID, status string, publishedAt, deprecatedAt *time.Time) error {
	q := db.New(pool)
	return q.UpdateArtifactStatus(ctx, db.UpdateArtifactStatusParams{
		ID:           id,
		Status:       status,
		PublishedAt:  publishedAt,
		DeprecatedAt: deprecatedAt,
	})
}

// IncrementArtifactEndorsementCount increments endorsement_count.
func IncrementArtifactEndorsementCount(ctx context.Context, pool *pgxpool.Pool, id uuid.UUID) error {
	q := db.New(pool)
	return q.IncrementArtifactEndorsementCount(ctx, id)
}

// IncrementArtifactAccess increments access_count and sets last_accessed.
func IncrementArtifactAccess(ctx context.Context, pool *pgxpool.Pool, id uuid.UUID) error {
	q := db.New(pool)
	return q.IncrementArtifactAccess(ctx, id)
}

// SnapshotArtifactVersion inserts a knowledge_history row.
func SnapshotArtifactVersion(ctx context.Context, pool *pgxpool.Pool, h *db.KnowledgeHistory) error {
	q := db.New(pool)
	return q.SnapshotArtifactVersion(ctx, db.SnapshotArtifactVersionParams{
		ArtifactID: h.ArtifactID,
		Version:    h.Version,
		Content:    h.Content,
		Summary:    h.Summary,
		ChangedBy:  h.ChangedBy,
		ChangeNote: h.ChangeNote,
	})
}

// GetArtifactHistory returns the version history for a knowledge artifact.
func GetArtifactHistory(ctx context.Context, pool *pgxpool.Pool, artifactID uuid.UUID) ([]*db.KnowledgeHistory, error) {
	q := db.New(pool)
	return q.GetArtifactHistory(ctx, artifactID)
}

// CreateEndorsement inserts a knowledge_endorsements row.
func CreateEndorsement(ctx context.Context, pool *pgxpool.Pool, artifactID, endorserID uuid.UUID, note *string) (*db.KnowledgeEndorsement, error) {
	q := db.New(pool)
	e, err := q.CreateEndorsement(ctx, db.CreateEndorsementParams{
		ArtifactID: artifactID,
		EndorserID: endorserID,
		Note:       note,
	})
	if err != nil {
		return nil, err
	}
	return e, nil
}

// GetEndorsementByEndorser finds an endorsement by (artifact, endorser). Returns nil, nil if not found.
func GetEndorsementByEndorser(ctx context.Context, pool *pgxpool.Pool, artifactID, endorserID uuid.UUID) (*db.KnowledgeEndorsement, error) {
	q := db.New(pool)
	e, err := q.GetEndorsementByEndorser(ctx, db.GetEndorsementByEndorserParams{
		ArtifactID: artifactID,
		EndorserID: endorserID,
	})
	if err == pgx.ErrNoRows {
		return nil, nil
	}
	return e, err
}

// SearchArtifacts filters artifacts by a text query (ILIKE on title/content), optional status, and optional scope.
// A zero scopeID means no scope filter.
func SearchArtifacts(ctx context.Context, pool *pgxpool.Pool, query, status string, scopeID uuid.UUID, limit, offset int) ([]*db.KnowledgeArtifact, error) {
	if limit < 0 || limit > math.MaxInt32 {
		return nil, fmt.Errorf("sharing: invalid limit: %d", limit)
	}
	if offset < 0 || offset > math.MaxInt32 {
		return nil, fmt.Errorf("sharing: invalid offset: %d", offset)
	}
	q := db.New(pool)
	rows, err := q.SearchArtifacts(ctx, db.SearchArtifactsParams{
		Title:   "%" + query + "%",
		Column2: status,
		Column3: scopeID,
		Limit:   int32(limit),
		Offset:  int32(offset),
	})
	if err != nil {
		return nil, fmt.Errorf("db: search artifacts: %w", err)
	}
	out := make([]*db.KnowledgeArtifact, len(rows))
	for i, r := range rows {
		out[i] = knowledgeArtifactFromSearchArtifactsRow(r)
	}
	return out, nil
}

// ListArtifactsByStatus returns artifacts filtered by status and optional scope.
// A zero scopeID means no scope filter.
func ListArtifactsByStatus(ctx context.Context, pool *pgxpool.Pool, status string, scopeID uuid.UUID, limit, offset int) ([]*db.KnowledgeArtifact, error) {
	if limit < 0 || limit > math.MaxInt32 {
		return nil, fmt.Errorf("sharing: invalid limit: %d", limit)
	}
	if offset < 0 || offset > math.MaxInt32 {
		return nil, fmt.Errorf("sharing: invalid offset: %d", offset)
	}
	q := db.New(pool)
	rows, err := q.ListArtifactsByStatus(ctx, db.ListArtifactsByStatusParams{
		Status:  status,
		Column2: scopeID,
		Limit:   int32(limit),
		Offset:  int32(offset),
	})
	if err != nil {
		return nil, fmt.Errorf("db: list artifacts by status: %w", err)
	}
	out := make([]*db.KnowledgeArtifact, len(rows))
	for i, r := range rows {
		out[i] = knowledgeArtifactFromListArtifactsByStatusRow(r)
	}
	return out, nil
}

// ListAllArtifacts returns artifacts for the given scope (zero scopeID = all scopes, admin view).
func ListAllArtifacts(ctx context.Context, pool *pgxpool.Pool, scopeID uuid.UUID, limit, offset int) ([]*db.KnowledgeArtifact, error) {
	if limit < 0 || limit > math.MaxInt32 {
		return nil, fmt.Errorf("sharing: invalid limit: %d", limit)
	}
	if offset < 0 || offset > math.MaxInt32 {
		return nil, fmt.Errorf("sharing: invalid offset: %d", offset)
	}
	q := db.New(pool)
	rows, err := q.ListAllArtifacts(ctx, db.ListAllArtifactsParams{
		Column1: scopeID,
		Limit:   int32(limit),
		Offset:  int32(offset),
	})
	if err != nil {
		return nil, fmt.Errorf("db: list all artifacts: %w", err)
	}
	out := make([]*db.KnowledgeArtifact, len(rows))
	for i, r := range rows {
		out[i] = knowledgeArtifactFromListAllArtifactsRow(r)
	}
	return out, nil
}

// ListVisibleArtifacts returns published artifacts visible to the given scope IDs.
func ListVisibleArtifacts(ctx context.Context, pool *pgxpool.Pool, callerScopeIDs []uuid.UUID, limit, offset int) ([]*db.KnowledgeArtifact, error) {
	q := db.New(pool)
	as, err := q.ListVisibleArtifacts(ctx, db.ListVisibleArtifactsParams{
		Column1: callerScopeIDs,
		Limit:   int32(limit),
		Offset:  int32(offset),
	})
	if err != nil {
		return nil, err
	}
	out := make([]*db.KnowledgeArtifact, len(as))
	for i, r := range as {
		out[i] = knowledgeArtifactFromListVisibleArtifactsRow(r)
	}
	return out, nil
}

// RecallArtifactsByVector retrieves published artifacts by vector similarity,
// resolving visibility (project/team/department/company/grants) from scopeID.
func RecallArtifactsByVector(ctx context.Context, pool *pgxpool.Pool, scopeID uuid.UUID, queryVec []float32, limit int, since, until *time.Time) ([]db.ArtifactScore, error) {
	q := db.New(pool)
	lower, upper := normalizeRecallWindowBounds(since, until)
	rows, err := q.RecallArtifactsByVector(ctx, db.RecallArtifactsByVectorParams{
		OwnerScopeID: scopeID,
		Limit:        int32(limit),
		Embedding:    vecPtr(queryVec),
		Column4:      lower,
		Column5:      upper,
	})
	if err != nil {
		return nil, err
	}
	results := make([]db.ArtifactScore, len(rows))
	for i, r := range rows {
		art := artifactFromRecallByVectorRow(r)
		results[i] = db.ArtifactScore{
			Artifact: art,
			VecScore: float64(r.VecScore),
		}
	}
	return results, nil
}

// RecallArtifactsByFTS retrieves published artifacts via full-text search,
// resolving visibility (project/team/department/company/grants) from scopeID.
func RecallArtifactsByFTS(ctx context.Context, pool *pgxpool.Pool, scopeID uuid.UUID, query string, limit int, since, until *time.Time) ([]db.ArtifactScore, error) {
	q := db.New(pool)
	lower, upper := normalizeRecallWindowBounds(since, until)
	rows, err := q.RecallArtifactsByFTS(ctx, db.RecallArtifactsByFTSParams{
		OwnerScopeID:   scopeID,
		Limit:          int32(limit),
		PlaintoTsquery: query,
		Column4:        lower,
		Column5:        upper,
	})
	if err != nil {
		return nil, err
	}
	results := make([]db.ArtifactScore, len(rows))
	for i, r := range rows {
		art := artifactFromRecallByFTSRow(r)
		results[i] = db.ArtifactScore{
			Artifact:  art,
			BM25Score: float64(r.Bm25Score),
		}
	}
	return results, nil
}

// RecallArtifactsByTrigram retrieves published artifacts via trigram similarity,
// resolving visibility (project/team/department/company/grants) from scopeID.
func RecallArtifactsByTrigram(ctx context.Context, pool *pgxpool.Pool, scopeID uuid.UUID, query string, limit int, since, until *time.Time) ([]db.ArtifactScore, error) {
	q := db.New(pool)
	lower, upper := normalizeRecallWindowBounds(since, until)
	rows, err := q.RecallArtifactsByTrigram(ctx, db.RecallArtifactsByTrigramParams{
		OwnerScopeID: scopeID,
		Limit:        int32(limit),
		Similarity:   query,
		Column4:      lower,
		Column5:      upper,
	})
	if err != nil {
		return nil, fmt.Errorf("db: recall artifacts by trigram: %w", err)
	}
	results := make([]db.ArtifactScore, len(rows))
	for i, r := range rows {
		art := artifactFromRecallByTrigramRow(r)
		results[i] = db.ArtifactScore{
			Artifact:  art,
			TrgmScore: float64(r.TrgmScore),
		}
	}
	return results, nil
}

// DeleteArtifactEntityLinks removes all artifact_entities rows for the given artifact.
func DeleteArtifactEntityLinks(ctx context.Context, pool *pgxpool.Pool, artifactID uuid.UUID) error {
	q := db.New(pool)
	return q.DeleteArtifactEntityLinks(ctx, artifactID)
}

// LinkArtifactToEntity inserts an artifact_entities row.
func LinkArtifactToEntity(ctx context.Context, pool *pgxpool.Pool, artifactID, entityID uuid.UUID, role string) error {
	q := db.New(pool)
	var rolePtr *string
	if role != "" {
		rolePtr = &role
	}
	err := q.LinkArtifactToEntity(ctx, db.LinkArtifactToEntityParams{
		ArtifactID: artifactID,
		EntityID:   entityID,
		Role:       rolePtr,
	})
	if err != nil {
		return fmt.Errorf("db: link artifact to entity: %w", err)
	}
	return nil
}

// InsertDigestSources records which source artifacts a digest was synthesised from.
func InsertDigestSources(ctx context.Context, pool *pgxpool.Pool, digestID uuid.UUID, sourceIDs []uuid.UUID) error {
	if len(sourceIDs) == 0 {
		return nil
	}
	batch := &pgx.Batch{}
	for _, sid := range sourceIDs {
		batch.Queue(
			`INSERT INTO artifact_digest_sources (digest_id, source_id) VALUES ($1, $2) ON CONFLICT DO NOTHING`,
			digestID, sid,
		)
	}
	return pool.SendBatch(ctx, batch).Close()
}

// ListDigestSources returns the source artifacts for a given digest artifact.
func ListDigestSources(ctx context.Context, pool *pgxpool.Pool, digestID uuid.UUID) ([]*db.KnowledgeArtifact, error) {
	rows, err := pool.Query(ctx, `
		SELECT ka.id, ka.knowledge_type, ka.owner_scope_id, ka.author_id,
		       ka.visibility, ka.status, ka.published_at, ka.deprecated_at,
		       ka.review_required, ka.title, ka.content, ka.summary,
		       ka.embedding, ka.embedding_model_id, ka.meta,
		       ka.endorsement_count, ka.access_count, ka.last_accessed,
		       ka.version, ka.previous_version, ka.source_memory_id, ka.source_ref,
		       ka.created_at, ka.updated_at
		FROM knowledge_artifacts ka
		JOIN artifact_digest_sources ads ON ads.source_id = ka.id
		WHERE ads.digest_id = $1
		ORDER BY ads.added_at ASC`, digestID)
	if err != nil {
		return nil, fmt.Errorf("db: list digest sources: %w", err)
	}
	defer rows.Close()
	return scanKnowledgeArtifactRows(rows)
}

// ListDigestsForSource returns published digests that cover a given source artifact.
func ListDigestsForSource(ctx context.Context, pool *pgxpool.Pool, sourceID uuid.UUID) ([]*db.KnowledgeArtifact, error) {
	rows, err := pool.Query(ctx, `
		SELECT ka.id, ka.knowledge_type, ka.owner_scope_id, ka.author_id,
		       ka.visibility, ka.status, ka.published_at, ka.deprecated_at,
		       ka.review_required, ka.title, ka.content, ka.summary,
		       ka.embedding, ka.embedding_model_id, ka.meta,
		       ka.endorsement_count, ka.access_count, ka.last_accessed,
		       ka.version, ka.previous_version, ka.source_memory_id, ka.source_ref,
		       ka.created_at, ka.updated_at
		FROM knowledge_artifacts ka
		JOIN artifact_digest_sources ads ON ads.digest_id = ka.id
		WHERE ads.source_id = $1
		  AND ka.status = 'published'
		ORDER BY ka.created_at DESC`, sourceID)
	if err != nil {
		return nil, fmt.Errorf("db: list digests for source: %w", err)
	}
	defer rows.Close()
	return scanKnowledgeArtifactRows(rows)
}

// GetSuppressedSourceIDs returns source artifact IDs that are covered by at
// least one published digest in digestIDs. Used for post-recall suppression.
func GetSuppressedSourceIDs(ctx context.Context, pool *pgxpool.Pool, digestIDs []uuid.UUID) (map[uuid.UUID]struct{}, error) {
	if len(digestIDs) == 0 {
		return nil, nil
	}
	rows, err := pool.Query(ctx, `
		SELECT ads.source_id
		FROM artifact_digest_sources ads
		JOIN knowledge_artifacts ka ON ka.id = ads.digest_id
		WHERE ads.digest_id = ANY($1)
		  AND ka.status = 'published'`, digestIDs)
	if err != nil {
		return nil, fmt.Errorf("db: get suppressed source ids: %w", err)
	}
	defer rows.Close()
	out := make(map[uuid.UUID]struct{})
	for rows.Next() {
		var id uuid.UUID
		if err := rows.Scan(&id); err != nil {
			return nil, fmt.Errorf("db: get suppressed source ids scan: %w", err)
		}
		out[id] = struct{}{}
	}
	return out, rows.Err()
}

// FlagDigestsStaleness inserts a staleness_flags row for every published digest
// that covers sourceID, skipping digests that already have an open flag with
// the same signal (ON CONFLICT DO NOTHING via the partial unique index).
func FlagDigestsStaleness(ctx context.Context, pool *pgxpool.Pool, sourceID uuid.UUID, signal string, confidence float64, evidence []byte) error {
	if evidence == nil {
		evidence = []byte("{}")
	}
	_, err := pool.Exec(ctx, `
		INSERT INTO staleness_flags (artifact_id, signal, confidence, evidence)
		SELECT ads.digest_id, $2, $3, $4
		FROM artifact_digest_sources ads
		JOIN knowledge_artifacts ka ON ka.id = ads.digest_id
		WHERE ads.source_id = $1
		  AND ka.status = 'published'
		  AND NOT EXISTS (
		      SELECT 1 FROM staleness_flags sf
		      WHERE sf.artifact_id = ads.digest_id
		        AND sf.signal = $2
		        AND sf.status = 'open'
		  )`,
		sourceID, signal, confidence, evidence)
	if err != nil {
		return fmt.Errorf("db: flag digests staleness: %w", err)
	}
	return nil
}

// InsertDigestLog records a synthesis audit entry.
func InsertDigestLog(ctx context.Context, pool *pgxpool.Pool, l *db.DigestLog) error {
	_, err := pool.Exec(ctx, `
		INSERT INTO knowledge_digest_log
		    (scope_id, digest_id, source_ids, strategy, synthesised_by)
		VALUES ($1, $2, $3, $4, $5)`,
		l.ScopeID, l.DigestID, l.SourceIDs, l.Strategy, l.SynthesisedBy)
	if err != nil {
		return fmt.Errorf("db: insert digest log: %w", err)
	}
	return nil
}

// scanKnowledgeArtifactRows is a shared scanner for knowledge artifact SELECT results.
func scanKnowledgeArtifactRows(rows pgx.Rows) ([]*db.KnowledgeArtifact, error) {
	var out []*db.KnowledgeArtifact
	for rows.Next() {
		var a db.KnowledgeArtifact
		if err := rows.Scan(
			&a.ID, &a.KnowledgeType, &a.OwnerScopeID, &a.AuthorID,
			&a.Visibility, &a.Status, &a.PublishedAt, &a.DeprecatedAt,
			&a.ReviewRequired, &a.Title, &a.Content, &a.Summary,
			&a.Embedding, &a.EmbeddingModelID, &a.Meta,
			&a.EndorsementCount, &a.AccessCount, &a.LastAccessed,
			&a.Version, &a.PreviousVersion, &a.SourceMemoryID, &a.SourceRef,
			&a.CreatedAt, &a.UpdatedAt,
		); err != nil {
			return nil, fmt.Errorf("db: scan knowledge artifact: %w", err)
		}
		out = append(out, &a)
	}
	return out, rows.Err()
}

// ── private row converters ────────────────────────────────────────────────────

func knowledgeArtifactFromCreateArtifactRow(r *db.CreateArtifactRow) *db.KnowledgeArtifact {
	return &db.KnowledgeArtifact{
		ID:               r.ID,
		KnowledgeType:    r.KnowledgeType,
		OwnerScopeID:     r.OwnerScopeID,
		AuthorID:         r.AuthorID,
		Visibility:       r.Visibility,
		Status:           r.Status,
		PublishedAt:      r.PublishedAt,
		DeprecatedAt:     r.DeprecatedAt,
		ReviewRequired:   r.ReviewRequired,
		Title:            r.Title,
		Content:          r.Content,
		Summary:          r.Summary,
		Embedding:        r.Embedding,
		EmbeddingModelID: r.EmbeddingModelID,
		Meta:             r.Meta,
		EndorsementCount: r.EndorsementCount,
		AccessCount:      r.AccessCount,
		LastAccessed:     r.LastAccessed,
		Version:          r.Version,
		PreviousVersion:  r.PreviousVersion,
		SourceMemoryID:   r.SourceMemoryID,
		SourceRef:        r.SourceRef,
		CreatedAt:        r.CreatedAt,
		UpdatedAt:        r.UpdatedAt,
		ArtifactKind:     r.ArtifactKind,
	}
}

func knowledgeArtifactFromGetArtifactRow(r *db.GetArtifactRow) *db.KnowledgeArtifact {
	return &db.KnowledgeArtifact{
		ID:               r.ID,
		KnowledgeType:    r.KnowledgeType,
		OwnerScopeID:     r.OwnerScopeID,
		AuthorID:         r.AuthorID,
		Visibility:       r.Visibility,
		Status:           r.Status,
		PublishedAt:      r.PublishedAt,
		DeprecatedAt:     r.DeprecatedAt,
		ReviewRequired:   r.ReviewRequired,
		Title:            r.Title,
		Content:          r.Content,
		Summary:          r.Summary,
		Embedding:        r.Embedding,
		EmbeddingModelID: r.EmbeddingModelID,
		Meta:             r.Meta,
		EndorsementCount: r.EndorsementCount,
		AccessCount:      r.AccessCount,
		LastAccessed:     r.LastAccessed,
		Version:          r.Version,
		PreviousVersion:  r.PreviousVersion,
		SourceMemoryID:   r.SourceMemoryID,
		SourceRef:        r.SourceRef,
		CreatedAt:        r.CreatedAt,
		UpdatedAt:        r.UpdatedAt,
		ArtifactKind:     r.ArtifactKind,
	}
}

func knowledgeArtifactFromUpdateArtifactRow(r *db.UpdateArtifactRow) *db.KnowledgeArtifact {
	return &db.KnowledgeArtifact{
		ID:               r.ID,
		KnowledgeType:    r.KnowledgeType,
		OwnerScopeID:     r.OwnerScopeID,
		AuthorID:         r.AuthorID,
		Visibility:       r.Visibility,
		Status:           r.Status,
		PublishedAt:      r.PublishedAt,
		DeprecatedAt:     r.DeprecatedAt,
		ReviewRequired:   r.ReviewRequired,
		Title:            r.Title,
		Content:          r.Content,
		Summary:          r.Summary,
		Embedding:        r.Embedding,
		EmbeddingModelID: r.EmbeddingModelID,
		Meta:             r.Meta,
		EndorsementCount: r.EndorsementCount,
		AccessCount:      r.AccessCount,
		LastAccessed:     r.LastAccessed,
		Version:          r.Version,
		PreviousVersion:  r.PreviousVersion,
		SourceMemoryID:   r.SourceMemoryID,
		SourceRef:        r.SourceRef,
		CreatedAt:        r.CreatedAt,
		UpdatedAt:        r.UpdatedAt,
		ArtifactKind:     r.ArtifactKind,
	}
}

func knowledgeArtifactFromSearchArtifactsRow(r *db.SearchArtifactsRow) *db.KnowledgeArtifact {
	return &db.KnowledgeArtifact{
		ID:               r.ID,
		KnowledgeType:    r.KnowledgeType,
		OwnerScopeID:     r.OwnerScopeID,
		AuthorID:         r.AuthorID,
		Visibility:       r.Visibility,
		Status:           r.Status,
		PublishedAt:      r.PublishedAt,
		DeprecatedAt:     r.DeprecatedAt,
		ReviewRequired:   r.ReviewRequired,
		Title:            r.Title,
		Content:          r.Content,
		Summary:          r.Summary,
		Embedding:        r.Embedding,
		EmbeddingModelID: r.EmbeddingModelID,
		Meta:             r.Meta,
		EndorsementCount: r.EndorsementCount,
		AccessCount:      r.AccessCount,
		LastAccessed:     r.LastAccessed,
		Version:          r.Version,
		PreviousVersion:  r.PreviousVersion,
		SourceMemoryID:   r.SourceMemoryID,
		SourceRef:        r.SourceRef,
		CreatedAt:        r.CreatedAt,
		UpdatedAt:        r.UpdatedAt,
		ArtifactKind:     r.ArtifactKind,
	}
}

func knowledgeArtifactFromListArtifactsByStatusRow(r *db.ListArtifactsByStatusRow) *db.KnowledgeArtifact {
	return &db.KnowledgeArtifact{
		ID:               r.ID,
		KnowledgeType:    r.KnowledgeType,
		OwnerScopeID:     r.OwnerScopeID,
		AuthorID:         r.AuthorID,
		Visibility:       r.Visibility,
		Status:           r.Status,
		PublishedAt:      r.PublishedAt,
		DeprecatedAt:     r.DeprecatedAt,
		ReviewRequired:   r.ReviewRequired,
		Title:            r.Title,
		Content:          r.Content,
		Summary:          r.Summary,
		Embedding:        r.Embedding,
		EmbeddingModelID: r.EmbeddingModelID,
		Meta:             r.Meta,
		EndorsementCount: r.EndorsementCount,
		AccessCount:      r.AccessCount,
		LastAccessed:     r.LastAccessed,
		Version:          r.Version,
		PreviousVersion:  r.PreviousVersion,
		SourceMemoryID:   r.SourceMemoryID,
		SourceRef:        r.SourceRef,
		CreatedAt:        r.CreatedAt,
		UpdatedAt:        r.UpdatedAt,
		ArtifactKind:     r.ArtifactKind,
	}
}

func knowledgeArtifactFromListAllArtifactsRow(r *db.ListAllArtifactsRow) *db.KnowledgeArtifact {
	return &db.KnowledgeArtifact{
		ID:               r.ID,
		KnowledgeType:    r.KnowledgeType,
		OwnerScopeID:     r.OwnerScopeID,
		AuthorID:         r.AuthorID,
		Visibility:       r.Visibility,
		Status:           r.Status,
		PublishedAt:      r.PublishedAt,
		DeprecatedAt:     r.DeprecatedAt,
		ReviewRequired:   r.ReviewRequired,
		Title:            r.Title,
		Content:          r.Content,
		Summary:          r.Summary,
		Embedding:        r.Embedding,
		EmbeddingModelID: r.EmbeddingModelID,
		Meta:             r.Meta,
		EndorsementCount: r.EndorsementCount,
		AccessCount:      r.AccessCount,
		LastAccessed:     r.LastAccessed,
		Version:          r.Version,
		PreviousVersion:  r.PreviousVersion,
		SourceMemoryID:   r.SourceMemoryID,
		SourceRef:        r.SourceRef,
		CreatedAt:        r.CreatedAt,
		UpdatedAt:        r.UpdatedAt,
		ArtifactKind:     r.ArtifactKind,
	}
}

func knowledgeArtifactFromListVisibleArtifactsRow(r *db.ListVisibleArtifactsRow) *db.KnowledgeArtifact {
	return &db.KnowledgeArtifact{
		ID:               r.ID,
		KnowledgeType:    r.KnowledgeType,
		OwnerScopeID:     r.OwnerScopeID,
		AuthorID:         r.AuthorID,
		Visibility:       r.Visibility,
		Status:           r.Status,
		PublishedAt:      r.PublishedAt,
		DeprecatedAt:     r.DeprecatedAt,
		ReviewRequired:   r.ReviewRequired,
		Title:            r.Title,
		Content:          r.Content,
		Summary:          r.Summary,
		Embedding:        r.Embedding,
		EmbeddingModelID: r.EmbeddingModelID,
		Meta:             r.Meta,
		EndorsementCount: r.EndorsementCount,
		AccessCount:      r.AccessCount,
		LastAccessed:     r.LastAccessed,
		Version:          r.Version,
		PreviousVersion:  r.PreviousVersion,
		SourceMemoryID:   r.SourceMemoryID,
		SourceRef:        r.SourceRef,
		CreatedAt:        r.CreatedAt,
		UpdatedAt:        r.UpdatedAt,
		ArtifactKind:     r.ArtifactKind,
	}
}

func knowledgeArtifactFromListCollectionItemsRow(r *db.ListCollectionItemsRow) *db.KnowledgeArtifact {
	return &db.KnowledgeArtifact{
		ID:               r.ID,
		KnowledgeType:    r.KnowledgeType,
		OwnerScopeID:     r.OwnerScopeID,
		AuthorID:         r.AuthorID,
		Visibility:       r.Visibility,
		Status:           r.Status,
		PublishedAt:      r.PublishedAt,
		DeprecatedAt:     r.DeprecatedAt,
		ReviewRequired:   r.ReviewRequired,
		Title:            r.Title,
		Content:          r.Content,
		Summary:          r.Summary,
		Embedding:        r.Embedding,
		EmbeddingModelID: r.EmbeddingModelID,
		Meta:             r.Meta,
		EndorsementCount: r.EndorsementCount,
		AccessCount:      r.AccessCount,
		LastAccessed:     r.LastAccessed,
		Version:          r.Version,
		PreviousVersion:  r.PreviousVersion,
		SourceMemoryID:   r.SourceMemoryID,
		SourceRef:        r.SourceRef,
		CreatedAt:        r.CreatedAt,
		UpdatedAt:        r.UpdatedAt,
		ArtifactKind:     r.ArtifactKind,
	}
}

func artifactFromRecallByVectorRow(r *db.RecallArtifactsByVectorRow) *db.KnowledgeArtifact {
	return &db.KnowledgeArtifact{
		ID:               r.ID,
		KnowledgeType:    r.KnowledgeType,
		OwnerScopeID:     r.OwnerScopeID,
		AuthorID:         r.AuthorID,
		Visibility:       r.Visibility,
		Status:           r.Status,
		PublishedAt:      r.PublishedAt,
		DeprecatedAt:     r.DeprecatedAt,
		ReviewRequired:   r.ReviewRequired,
		Title:            r.Title,
		Content:          r.Content,
		Summary:          r.Summary,
		Embedding:        r.Embedding,
		EmbeddingModelID: r.EmbeddingModelID,
		Meta:             r.Meta,
		EndorsementCount: r.EndorsementCount,
		AccessCount:      r.AccessCount,
		LastAccessed:     r.LastAccessed,
		Version:          r.Version,
		PreviousVersion:  r.PreviousVersion,
		SourceMemoryID:   r.SourceMemoryID,
		SourceRef:        r.SourceRef,
		CreatedAt:        r.CreatedAt,
		UpdatedAt:        r.UpdatedAt,
	}
}

func artifactFromRecallByFTSRow(r *db.RecallArtifactsByFTSRow) *db.KnowledgeArtifact {
	return &db.KnowledgeArtifact{
		ID:               r.ID,
		KnowledgeType:    r.KnowledgeType,
		OwnerScopeID:     r.OwnerScopeID,
		AuthorID:         r.AuthorID,
		Visibility:       r.Visibility,
		Status:           r.Status,
		PublishedAt:      r.PublishedAt,
		DeprecatedAt:     r.DeprecatedAt,
		ReviewRequired:   r.ReviewRequired,
		Title:            r.Title,
		Content:          r.Content,
		Summary:          r.Summary,
		Embedding:        r.Embedding,
		EmbeddingModelID: r.EmbeddingModelID,
		Meta:             r.Meta,
		EndorsementCount: r.EndorsementCount,
		AccessCount:      r.AccessCount,
		LastAccessed:     r.LastAccessed,
		Version:          r.Version,
		PreviousVersion:  r.PreviousVersion,
		SourceMemoryID:   r.SourceMemoryID,
		SourceRef:        r.SourceRef,
		CreatedAt:        r.CreatedAt,
		UpdatedAt:        r.UpdatedAt,
	}
}

func artifactFromRecallByTrigramRow(r *db.RecallArtifactsByTrigramRow) *db.KnowledgeArtifact {
	return &db.KnowledgeArtifact{
		ID:               r.ID,
		KnowledgeType:    r.KnowledgeType,
		OwnerScopeID:     r.OwnerScopeID,
		AuthorID:         r.AuthorID,
		Visibility:       r.Visibility,
		Status:           r.Status,
		PublishedAt:      r.PublishedAt,
		DeprecatedAt:     r.DeprecatedAt,
		ReviewRequired:   r.ReviewRequired,
		Title:            r.Title,
		Content:          r.Content,
		Summary:          r.Summary,
		Embedding:        r.Embedding,
		EmbeddingModelID: r.EmbeddingModelID,
		Meta:             r.Meta,
		EndorsementCount: r.EndorsementCount,
		AccessCount:      r.AccessCount,
		LastAccessed:     r.LastAccessed,
		Version:          r.Version,
		PreviousVersion:  r.PreviousVersion,
		SourceMemoryID:   r.SourceMemoryID,
		SourceRef:        r.SourceRef,
		CreatedAt:        r.CreatedAt,
		UpdatedAt:        r.UpdatedAt,
	}
}
