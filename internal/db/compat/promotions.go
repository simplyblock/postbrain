package compat

import (
	"context"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/simplyblock/postbrain/internal/db"
)

// CreatePromotionRequest inserts a new promotion_requests row.
func CreatePromotionRequest(ctx context.Context, pool *pgxpool.Pool, req *db.PromotionRequest) (*db.PromotionRequest, error) {
	q := db.New(pool)
	result, err := q.CreatePromotionRequest(ctx, db.CreatePromotionRequestParams{
		MemoryID:             req.MemoryID,
		RequestedBy:          req.RequestedBy,
		TargetScopeID:        req.TargetScopeID,
		TargetVisibility:     req.TargetVisibility,
		ProposedTitle:        req.ProposedTitle,
		ProposedCollectionID: req.ProposedCollectionID,
	})
	if err != nil {
		return nil, fmt.Errorf("db: create promotion request: %w", err)
	}
	return result, nil
}

// GetPromotionRequest retrieves a promotion request by ID. Returns nil, nil if not found.
func GetPromotionRequest(ctx context.Context, pool *pgxpool.Pool, id uuid.UUID) (*db.PromotionRequest, error) {
	q := db.New(pool)
	p, err := q.GetPromotionRequest(ctx, id)
	if err == pgx.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("db: get promotion request: %w", err)
	}
	return p, nil
}

// ListPendingPromotions returns pending promotion requests.
// When targetScopeID is the zero UUID, all pending promotions across all scopes are returned.
func ListPendingPromotions(ctx context.Context, pool *pgxpool.Pool, targetScopeID uuid.UUID) ([]*db.PromotionRequest, error) {
	if targetScopeID == (uuid.UUID{}) {
		rows, err := pool.Query(ctx,
			`SELECT id, memory_id, requested_by, target_scope_id, target_visibility,
			        proposed_title, proposed_collection_id, status, reviewer_id, review_note,
			        reviewed_at, result_artifact_id, created_at
			 FROM promotion_requests WHERE status='pending' ORDER BY created_at`,
		)
		if err != nil {
			return nil, fmt.Errorf("db: list all pending promotions: %w", err)
		}
		defer rows.Close()
		var ps []*db.PromotionRequest
		for rows.Next() {
			var p db.PromotionRequest
			if err := rows.Scan(
				&p.ID, &p.MemoryID, &p.RequestedBy, &p.TargetScopeID, &p.TargetVisibility,
				&p.ProposedTitle, &p.ProposedCollectionID, &p.Status, &p.ReviewerID, &p.ReviewNote,
				&p.ReviewedAt, &p.ResultArtifactID, &p.CreatedAt,
			); err != nil {
				return nil, fmt.Errorf("db: list all pending promotions scan: %w", err)
			}
			ps = append(ps, &p)
		}
		return ps, rows.Err()
	}
	q := db.New(pool)
	ps, err := q.ListPendingPromotions(ctx, targetScopeID)
	if err != nil {
		return nil, err
	}
	return ps, nil
}

// ListPromotions returns promotion requests, optionally filtered by scope and/or status.
// Pass zero UUID to query all scopes. Pass empty status to query all statuses.
func ListPromotions(ctx context.Context, pool *pgxpool.Pool, targetScopeID uuid.UUID, status string, limit int) ([]*db.PromotionRequest, error) {
	if limit <= 0 {
		limit = 200
	}
	base := `SELECT id, memory_id, requested_by, target_scope_id, target_visibility,
	        proposed_title, proposed_collection_id, status, reviewer_id, review_note,
	        reviewed_at, result_artifact_id, created_at
	 FROM promotion_requests`
	order := ` ORDER BY created_at DESC LIMIT $1`
	args := []any{limit}

	query := base
	switch {
	case targetScopeID == (uuid.UUID{}) && status == "":
		// all scopes, all statuses
	case targetScopeID == (uuid.UUID{}):
		query += ` WHERE status = $2`
		args = append(args, status)
	case status == "":
		query += ` WHERE target_scope_id = $2`
		args = append(args, targetScopeID)
	default:
		query += ` WHERE target_scope_id = $2 AND status = $3`
		args = append(args, targetScopeID, status)
	}
	query += order

	rows, err := pool.Query(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("db: list promotions: %w", err)
	}
	defer rows.Close()

	var ps []*db.PromotionRequest
	for rows.Next() {
		var p db.PromotionRequest
		if err := rows.Scan(
			&p.ID, &p.MemoryID, &p.RequestedBy, &p.TargetScopeID, &p.TargetVisibility,
			&p.ProposedTitle, &p.ProposedCollectionID, &p.Status, &p.ReviewerID, &p.ReviewNote,
			&p.ReviewedAt, &p.ResultArtifactID, &p.CreatedAt,
		); err != nil {
			return nil, fmt.Errorf("db: list promotions scan: %w", err)
		}
		ps = append(ps, &p)
	}
	return ps, rows.Err()
}
