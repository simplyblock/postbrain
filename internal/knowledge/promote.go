package knowledge

import (
	"context"
	"errors"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/simplyblock/postbrain/internal/db"
	"github.com/simplyblock/postbrain/internal/embedding"
)

// ErrAlreadyPromoted is returned when a memory is already nominated or promoted.
var ErrAlreadyPromoted = errors.New("knowledge: memory is already nominated or promoted")

// promoterDB abstracts the DB calls used by Promoter so it can be unit-tested.
type promoterDB interface {
	getMemory(ctx context.Context, id uuid.UUID) (*db.Memory, error)
	createPromotionRequest(ctx context.Context, req *db.PromotionRequest) (*db.PromotionRequest, error)
	markMemoryNominated(ctx context.Context, memoryID uuid.UUID) error
}

// poolPromoterDB wraps a pgxpool.Pool to implement promoterDB.
type poolPromoterDB struct {
	pool *pgxpool.Pool
}

func (p *poolPromoterDB) getMemory(ctx context.Context, id uuid.UUID) (*db.Memory, error) {
	return db.GetMemory(ctx, p.pool, id)
}

func (p *poolPromoterDB) createPromotionRequest(ctx context.Context, req *db.PromotionRequest) (*db.PromotionRequest, error) {
	return db.CreatePromotionRequest(ctx, p.pool, req)
}

func (p *poolPromoterDB) markMemoryNominated(ctx context.Context, memoryID uuid.UUID) error {
	_, err := p.pool.Exec(ctx,
		`UPDATE memories SET promotion_status='nominated', updated_at=now() WHERE id=$1`,
		memoryID,
	)
	return err
}

// Promoter manages the memory → knowledge promotion workflow.
type Promoter struct {
	pool  *pgxpool.Pool
	svc   embeddingService
	dbOps promoterDB
}

// NewPromoter creates a new Promoter backed by the given pool and embedding service.
func NewPromoter(pool *pgxpool.Pool, svc *embedding.EmbeddingService) *Promoter {
	return &Promoter{
		pool:  pool,
		svc:   &embeddingServiceAdapter{svc: svc},
		dbOps: &poolPromoterDB{pool: pool},
	}
}

// PromoteInput holds the fields required to create a promotion request.
type PromoteInput struct {
	MemoryID             uuid.UUID
	RequestedBy          uuid.UUID
	TargetScopeID        uuid.UUID
	TargetVisibility     string
	ProposedTitle        *string
	ProposedCollectionID *uuid.UUID
}

// CreateRequest creates a promotion request and marks the memory as "nominated".
func (p *Promoter) CreateRequest(ctx context.Context, input PromoteInput) (*db.PromotionRequest, error) {
	memory, err := p.dbOps.getMemory(ctx, input.MemoryID)
	if err != nil {
		return nil, fmt.Errorf("knowledge: promote get memory: %w", err)
	}
	if memory == nil {
		return nil, fmt.Errorf("knowledge: promote: memory %s not found", input.MemoryID)
	}
	if memory.PromotionStatus == "nominated" || memory.PromotionStatus == "promoted" {
		return nil, ErrAlreadyPromoted
	}

	req := &db.PromotionRequest{
		MemoryID:             input.MemoryID,
		RequestedBy:          input.RequestedBy,
		TargetScopeID:        input.TargetScopeID,
		TargetVisibility:     input.TargetVisibility,
		ProposedTitle:        input.ProposedTitle,
		ProposedCollectionID: input.ProposedCollectionID,
	}
	created, err := p.dbOps.createPromotionRequest(ctx, req)
	if err != nil {
		return nil, fmt.Errorf("knowledge: promote create request: %w", err)
	}

	if err := p.dbOps.markMemoryNominated(ctx, input.MemoryID); err != nil {
		return nil, fmt.Errorf("knowledge: promote mark nominated: %w", err)
	}

	return created, nil
}

// Approve executes the 5-step atomic promotion transaction.
// Steps:
//  1. Create draft artifact from memory content
//  2. Set promotion request result_artifact_id
//  3. Set promotion request status=approved
//  4. Set memory.promoted_to = artifact.id
//  5. Set memory.promotion_status='promoted'
func (p *Promoter) Approve(ctx context.Context, requestID, reviewerID uuid.UUID, callerID uuid.UUID) (*db.KnowledgeArtifact, error) {
	tx, err := p.pool.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.Serializable})
	if err != nil {
		return nil, fmt.Errorf("knowledge: approve begin tx: %w", err)
	}
	defer tx.Rollback(ctx) //nolint:errcheck

	// 1. Get promotion request.
	var req db.PromotionRequest
	if err := tx.QueryRow(ctx,
		`SELECT id, memory_id, requested_by, target_scope_id, target_visibility,
		        proposed_title, proposed_collection_id, status
		 FROM promotion_requests WHERE id=$1`,
		requestID,
	).Scan(
		&req.ID, &req.MemoryID, &req.RequestedBy, &req.TargetScopeID, &req.TargetVisibility,
		&req.ProposedTitle, &req.ProposedCollectionID, &req.Status,
	); err != nil {
		return nil, fmt.Errorf("knowledge: approve get request: %w", err)
	}

	// 2. Get memory.
	var mem db.Memory
	if err := tx.QueryRow(ctx,
		`SELECT id, memory_type, scope_id, author_id, content, summary, source_ref
		 FROM memories WHERE id=$1`,
		req.MemoryID,
	).Scan(
		&mem.ID, &mem.MemoryType, &mem.ScopeID, &mem.AuthorID, &mem.Content, &mem.Summary, &mem.SourceRef,
	); err != nil {
		return nil, fmt.Errorf("knowledge: approve get memory: %w", err)
	}

	// 3. Embed content.
	var embeddingVec []float32
	if p.svc != nil {
		embeddingVec, err = p.svc.EmbedText(ctx, mem.Content)
		if err != nil {
			return nil, fmt.Errorf("knowledge: approve embed: %w", err)
		}
	}

	// 4. Create draft artifact.
	title := mem.Content
	if req.ProposedTitle != nil {
		title = *req.ProposedTitle
	}
	meta := []byte("{}")
	var embeddingVal interface{}
	if len(embeddingVec) > 0 {
		embeddingVal = db.ExportFloat32SliceToVector(embeddingVec)
	}
	var artifact db.KnowledgeArtifact
	if err := tx.QueryRow(ctx,
		`INSERT INTO knowledge_artifacts
		 (knowledge_type, owner_scope_id, author_id, visibility, status,
		  review_required, title, content, summary, embedding, meta,
		  version, source_memory_id, source_ref)
		 VALUES ($1,$2,$3,$4,'draft',1,$5,$6,$7,$8::vector,$9,1,$10,$11)
		 RETURNING id, knowledge_type, owner_scope_id, author_id,
		           visibility, status, published_at, deprecated_at, review_required,
		           title, content, summary, embedding::text, embedding_model_id, meta,
		           endorsement_count, access_count, last_accessed,
		           version, previous_version, source_memory_id, source_ref,
		           created_at, updated_at`,
		mem.MemoryType, req.TargetScopeID, mem.AuthorID, req.TargetVisibility,
		title, mem.Content, mem.Summary, embeddingVal, meta,
		mem.ID, mem.SourceRef,
	).Scan(
		&artifact.ID, &artifact.KnowledgeType, &artifact.OwnerScopeID, &artifact.AuthorID,
		&artifact.Visibility, &artifact.Status, &artifact.PublishedAt, &artifact.DeprecatedAt, &artifact.ReviewRequired,
		&artifact.Title, &artifact.Content, &artifact.Summary, new(*string), &artifact.EmbeddingModelID, &artifact.Meta,
		&artifact.EndorsementCount, &artifact.AccessCount, &artifact.LastAccessed,
		&artifact.Version, &artifact.PreviousVersion, &artifact.SourceMemoryID, &artifact.SourceRef,
		&artifact.CreatedAt, &artifact.UpdatedAt,
	); err != nil {
		return nil, fmt.Errorf("knowledge: approve create artifact: %w", err)
	}

	// 5. Update promotion request.
	if _, err := tx.Exec(ctx,
		`UPDATE promotion_requests
		 SET result_artifact_id=$2, status='approved', reviewer_id=$3, reviewed_at=now()
		 WHERE id=$1`,
		requestID, artifact.ID, reviewerID,
	); err != nil {
		return nil, fmt.Errorf("knowledge: approve update request: %w", err)
	}

	// 6. Update memory.
	if _, err := tx.Exec(ctx,
		`UPDATE memories SET promoted_to=$2, promotion_status='promoted', updated_at=now() WHERE id=$1`,
		mem.ID, artifact.ID,
	); err != nil {
		return nil, fmt.Errorf("knowledge: approve update memory: %w", err)
	}

	if err := tx.Commit(ctx); err != nil {
		return nil, fmt.Errorf("knowledge: approve commit: %w", err)
	}
	return &artifact, nil
}

// Reject sets the promotion request to rejected.
func (p *Promoter) Reject(ctx context.Context, requestID, reviewerID uuid.UUID, note *string) error {
	_, err := p.pool.Exec(ctx,
		`UPDATE promotion_requests
		 SET status='rejected', reviewer_id=$2, review_note=$3, reviewed_at=now()
		 WHERE id=$1`,
		requestID, reviewerID, note,
	)
	if err != nil {
		return fmt.Errorf("knowledge: reject: %w", err)
	}
	return nil
}
