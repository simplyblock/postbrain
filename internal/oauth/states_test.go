package oauth

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/simplyblock/postbrain/internal/db"
)

type fakeStateQueries struct {
	rows map[string]*db.OauthState
	now  func() time.Time
}

func (f *fakeStateQueries) IssueState(_ context.Context, arg db.IssueStateParams) (*db.OauthState, error) {
	row := &db.OauthState{
		ID:        uuid.New(),
		StateHash: arg.StateHash,
		Kind:      arg.Kind,
		Payload:   arg.Payload,
		ExpiresAt: arg.ExpiresAt,
		CreatedAt: f.now(),
	}
	f.rows[arg.StateHash] = row
	return row, nil
}

func (f *fakeStateQueries) ConsumeState(_ context.Context, stateHash string) (*db.OauthState, error) {
	row, ok := f.rows[stateHash]
	if !ok {
		return nil, pgx.ErrNoRows
	}
	if row.UsedAt != nil || !row.ExpiresAt.After(f.now()) {
		return nil, pgx.ErrNoRows
	}
	usedAt := f.now()
	row.UsedAt = &usedAt
	return row, nil
}

func newTestStateStore(now time.Time) (*StateStore, *fakeStateQueries) {
	fake := &fakeStateQueries{
		rows: map[string]*db.OauthState{},
		now:  func() time.Time { return now },
	}
	store := &StateStore{
		q:   fake,
		now: func() time.Time { return now },
	}
	return store, fake
}

func TestStateStore_Issue_RoundTrip(t *testing.T) {
	now := time.Date(2026, 4, 2, 10, 0, 0, 0, time.UTC)
	store, _ := newTestStateStore(now)
	ctx := context.Background()
	payload := map[string]any{"client_id": "pb_client_1", "approved": true}

	rawState, err := store.Issue(ctx, "mcp_consent", payload, 15*time.Minute)
	if err != nil {
		t.Fatalf("Issue: %v", err)
	}
	got, err := store.Consume(ctx, rawState)
	if err != nil {
		t.Fatalf("Consume: %v", err)
	}

	if got.Payload["client_id"] != "pb_client_1" {
		t.Fatalf("payload client_id = %v, want pb_client_1", got.Payload["client_id"])
	}
	if got.Payload["approved"] != true {
		t.Fatalf("payload approved = %v, want true", got.Payload["approved"])
	}
}

func TestStateStore_Consume_SecondCall_ReturnsNotFound(t *testing.T) {
	now := time.Date(2026, 4, 2, 10, 0, 0, 0, time.UTC)
	store, _ := newTestStateStore(now)
	ctx := context.Background()

	rawState, err := store.Issue(ctx, "social", map[string]any{}, 15*time.Minute)
	if err != nil {
		t.Fatalf("Issue: %v", err)
	}
	if _, err := store.Consume(ctx, rawState); err != nil {
		t.Fatalf("first Consume: %v", err)
	}
	if _, err := store.Consume(ctx, rawState); !errors.Is(err, ErrNotFound) {
		t.Fatalf("second Consume error = %v, want ErrNotFound", err)
	}
}

func TestStateStore_Consume_Expired_ReturnsNotFound(t *testing.T) {
	now := time.Date(2026, 4, 2, 10, 0, 0, 0, time.UTC)
	store, fake := newTestStateStore(now)
	ctx := context.Background()

	rawState := "expired-state"
	fake.rows[hashSHA256Hex(rawState)] = &db.OauthState{
		ID:        uuid.New(),
		StateHash: hashSHA256Hex(rawState),
		Kind:      "social",
		Payload:   []byte(`{}`),
		ExpiresAt: now.Add(-time.Minute),
		CreatedAt: now.Add(-2 * time.Minute),
	}

	if _, err := store.Consume(ctx, rawState); !errors.Is(err, ErrNotFound) {
		t.Fatalf("Consume expired error = %v, want ErrNotFound", err)
	}
}

func TestStateStore_Issue_HashesRawState(t *testing.T) {
	now := time.Date(2026, 4, 2, 10, 0, 0, 0, time.UTC)
	store, fake := newTestStateStore(now)
	ctx := context.Background()

	rawState, err := store.Issue(ctx, "social", map[string]any{}, 15*time.Minute)
	if err != nil {
		t.Fatalf("Issue: %v", err)
	}
	if _, ok := fake.rows[rawState]; ok {
		t.Fatal("raw state should not be used as storage key")
	}
	if _, ok := fake.rows[hashSHA256Hex(rawState)]; !ok {
		t.Fatal("expected hashed state key to be stored")
	}
}
