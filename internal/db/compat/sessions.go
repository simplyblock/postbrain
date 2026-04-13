package compat

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/simplyblock/postbrain/internal/db"
)

// CreateSession inserts a new session row.
func CreateSession(ctx context.Context, pool *pgxpool.Pool, scopeID, principalID uuid.UUID, meta []byte) (*db.Session, error) {
	if meta == nil {
		meta = []byte("{}")
	}
	q := db.New(pool)
	return q.CreateSession(ctx, db.CreateSessionParams{
		ScopeID:     scopeID,
		PrincipalID: &principalID,
		Meta:        meta,
	})
}

// GetSession retrieves a session by ID. Returns nil, nil if not found.
func GetSession(ctx context.Context, pool *pgxpool.Pool, id uuid.UUID) (*db.Session, error) {
	q := db.New(pool)
	s, err := q.GetSession(ctx, id)
	if err == pgx.ErrNoRows {
		return nil, nil
	}
	return s, err
}

// EndSession marks a session as ended, optionally merging meta.
func EndSession(ctx context.Context, pool *pgxpool.Pool, id uuid.UUID, meta []byte) (*db.Session, error) {
	q := db.New(pool)
	return q.EndSession(ctx, db.EndSessionParams{
		ID:      id,
		Column2: time.Now().UTC(),
		Column3: meta,
	})
}

// InsertEvent appends a typed event to the partitioned events table.
// sessionID may be uuid.Nil when no session context is available.
func InsertEvent(ctx context.Context, pool *pgxpool.Pool, sessionID, scopeID uuid.UUID, eventType string, payload []byte) error {
	if payload == nil {
		payload = []byte("{}")
	}
	_, err := pool.Exec(ctx,
		`INSERT INTO events (session_id, scope_id, event_type, payload) VALUES ($1,$2,$3,$4)`,
		sessionID, scopeID, eventType, payload,
	)
	if err != nil {
		return fmt.Errorf("db: insert event: %w", err)
	}
	return nil
}
