package oauth

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/simplyblock/postbrain/internal/db"
)

var ErrNotFound = errors.New("oauth: state not found")

type stateQueries interface {
	IssueState(ctx context.Context, arg db.IssueStateParams) (*db.OauthState, error)
	ConsumeState(ctx context.Context, stateHash string) (*db.OauthState, error)
	GetStateByHash(ctx context.Context, stateHash string) (*db.OauthState, error)
}

// StateRecord is a consumed oauth_states record.
type StateRecord struct {
	ID        uuid.UUID
	Kind      string
	Payload   map[string]any
	ExpiresAt time.Time
	UsedAt    *time.Time
	CreatedAt time.Time
}

// StateStore persists and consumes hashed OAuth state values.
type StateStore struct {
	q   stateQueries
	now func() time.Time
}

// NewStateStore builds a StateStore backed by sqlc queries.
func NewStateStore(pool *pgxpool.Pool) *StateStore {
	return &StateStore{
		q:   db.New(pool),
		now: time.Now,
	}
}

// Issue creates a random raw state, stores only its SHA-256 hash, and returns raw state.
func (s *StateStore) Issue(ctx context.Context, kind string, payload map[string]any, ttl time.Duration) (string, error) {
	rawState, err := generateRawState()
	if err != nil {
		return "", err
	}
	stateHash := hashSHA256Hex(rawState)

	if payload == nil {
		payload = map[string]any{}
	}
	payloadJSON, err := json.Marshal(payload)
	if err != nil {
		return "", fmt.Errorf("marshal state payload: %w", err)
	}

	_, err = s.q.IssueState(ctx, db.IssueStateParams{
		StateHash: stateHash,
		Kind:      kind,
		Payload:   payloadJSON,
		ExpiresAt: s.now().Add(ttl),
	})
	if err != nil {
		return "", err
	}
	return rawState, nil
}

// Consume resolves and marks a state as used.
func (s *StateStore) Consume(ctx context.Context, rawState string) (*StateRecord, error) {
	row, err := s.q.ConsumeState(ctx, hashSHA256Hex(rawState))
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, err
	}

	payload := map[string]any{}
	if len(row.Payload) > 0 {
		if err := json.Unmarshal(row.Payload, &payload); err != nil {
			return nil, fmt.Errorf("unmarshal state payload: %w", err)
		}
	}

	return &StateRecord{
		ID:        row.ID,
		Kind:      row.Kind,
		Payload:   payload,
		ExpiresAt: row.ExpiresAt,
		UsedAt:    row.UsedAt,
		CreatedAt: row.CreatedAt,
	}, nil
}

// Peek resolves a not-yet-consumed state without marking it used.
func (s *StateStore) Peek(ctx context.Context, rawState string) (*StateRecord, error) {
	row, err := s.q.GetStateByHash(ctx, hashSHA256Hex(rawState))
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, err
	}

	payload := map[string]any{}
	if len(row.Payload) > 0 {
		if err := json.Unmarshal(row.Payload, &payload); err != nil {
			return nil, fmt.Errorf("unmarshal state payload: %w", err)
		}
	}

	return &StateRecord{
		ID:        row.ID,
		Kind:      row.Kind,
		Payload:   payload,
		ExpiresAt: row.ExpiresAt,
		UsedAt:    row.UsedAt,
		CreatedAt: row.CreatedAt,
	}, nil
}

func generateRawState() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", fmt.Errorf("generate state: %w", err)
	}
	return base64.RawURLEncoding.EncodeToString(b), nil
}

func hashSHA256Hex(raw string) string {
	sum := sha256.Sum256([]byte(raw))
	return hex.EncodeToString(sum[:])
}
