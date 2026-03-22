//go:build integration

package principals_test

import (
	"context"
	"os"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/simplyblock/postbrain/internal/config"
	"github.com/simplyblock/postbrain/internal/db"
	"github.com/simplyblock/postbrain/internal/principals"
)

func testPool(t *testing.T) *pgxpool.Pool {
	t.Helper()
	dsn := os.Getenv("TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("TEST_DATABASE_URL not set; skipping integration test")
	}
	cfg := &config.DatabaseConfig{
		URL:            dsn,
		MaxOpen:        5,
		MaxIdle:        1,
		ConnectTimeout: 5000000000, // 5s
	}
	pool, err := db.NewPool(context.Background(), cfg)
	if err != nil {
		t.Fatalf("newPool: %v", err)
	}
	t.Cleanup(func() { pool.Close() })
	return pool
}

func TestStoreCreate(t *testing.T) {
	pool := testPool(t)
	s := principals.NewStore(pool)

	p, err := s.Create(context.Background(), "user", "test-alice", "Alice Test", []byte(`{"email":"alice@test.example"}`))
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if p.Slug != "test-alice" {
		t.Errorf("slug = %q; want %q", p.Slug, "test-alice")
	}
	if p.Kind != "user" {
		t.Errorf("kind = %q; want %q", p.Kind, "user")
	}

	// Cleanup
	if delErr := s.Delete(context.Background(), p.ID); delErr != nil {
		t.Logf("cleanup delete: %v", delErr)
	}
}

func TestStoreGetByID(t *testing.T) {
	pool := testPool(t)
	s := principals.NewStore(pool)

	created, err := s.Create(context.Background(), "agent", "test-bot-1", "Test Bot", nil)
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	t.Cleanup(func() { _ = s.Delete(context.Background(), created.ID) })

	got, err := s.GetByID(context.Background(), created.ID)
	if err != nil {
		t.Fatalf("GetByID: %v", err)
	}
	if got == nil {
		t.Fatal("GetByID returned nil")
	}
	if got.ID != created.ID {
		t.Errorf("id mismatch: got %v want %v", got.ID, created.ID)
	}
}

func TestStoreGetBySlug(t *testing.T) {
	pool := testPool(t)
	s := principals.NewStore(pool)

	created, err := s.Create(context.Background(), "team", "test-team-xyz", "Test Team XYZ", nil)
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	t.Cleanup(func() { _ = s.Delete(context.Background(), created.ID) })

	got, err := s.GetBySlug(context.Background(), "test-team-xyz")
	if err != nil {
		t.Fatalf("GetBySlug: %v", err)
	}
	if got == nil {
		t.Fatal("GetBySlug returned nil")
	}
	if got.Slug != "test-team-xyz" {
		t.Errorf("slug = %q; want %q", got.Slug, "test-team-xyz")
	}
}

func TestStoreUpdate(t *testing.T) {
	pool := testPool(t)
	s := principals.NewStore(pool)

	created, err := s.Create(context.Background(), "user", "test-update-user", "Original Name", nil)
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	t.Cleanup(func() { _ = s.Delete(context.Background(), created.ID) })

	updated, err := s.Update(context.Background(), created.ID, "Updated Name", []byte(`{"updated":true}`))
	if err != nil {
		t.Fatalf("Update: %v", err)
	}
	if updated.DisplayName != "Updated Name" {
		t.Errorf("display_name = %q; want %q", updated.DisplayName, "Updated Name")
	}
}
