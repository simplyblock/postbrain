//go:build integration

package principals_test

import (
	"context"
	"testing"

	"github.com/google/uuid"

	"github.com/simplyblock/postbrain/internal/principals"
	"github.com/simplyblock/postbrain/internal/testhelper"
)

func TestStore_Create(t *testing.T) {
	t.Parallel()
	pool := testhelper.NewTestPool(t)
	s := principals.NewStore(pool)
	ctx := context.Background()

	p, err := s.Create(ctx, "user", "store-create-alice", "Alice", []byte(`{"email":"alice@test.example"}`))
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if p.Slug != "store-create-alice" {
		t.Errorf("slug = %q; want %q", p.Slug, "store-create-alice")
	}
	if p.Kind != "user" {
		t.Errorf("kind = %q; want %q", p.Kind, "user")
	}
}

func TestStore_GetByID(t *testing.T) {
	t.Parallel()
	pool := testhelper.NewTestPool(t)
	s := principals.NewStore(pool)
	ctx := context.Background()

	created, err := s.Create(ctx, "agent", "store-getbyid-bot", "Bot", nil)
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	got, err := s.GetByID(ctx, created.ID)
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

func TestStore_GetByID_NotFound(t *testing.T) {
	t.Parallel()
	pool := testhelper.NewTestPool(t)
	s := principals.NewStore(pool)

	got, err := s.GetByID(context.Background(), uuid.New())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != nil {
		t.Errorf("expected nil for unknown ID, got %+v", got)
	}
}

func TestStore_GetBySlug(t *testing.T) {
	t.Parallel()
	pool := testhelper.NewTestPool(t)
	s := principals.NewStore(pool)
	ctx := context.Background()

	created, err := s.Create(ctx, "team", "store-getbyslug-team", "Team", nil)
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	got, err := s.GetBySlug(ctx, created.Slug)
	if err != nil {
		t.Fatalf("GetBySlug: %v", err)
	}
	if got == nil {
		t.Fatal("GetBySlug returned nil")
	}
	if got.Slug != created.Slug {
		t.Errorf("slug = %q; want %q", got.Slug, created.Slug)
	}
}

func TestStore_Update(t *testing.T) {
	t.Parallel()
	pool := testhelper.NewTestPool(t)
	s := principals.NewStore(pool)
	ctx := context.Background()

	created, err := s.Create(ctx, "user", "store-update-user", "Original", nil)
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	updated, err := s.Update(ctx, created.ID, "Updated Name", []byte(`{"updated":true}`))
	if err != nil {
		t.Fatalf("Update: %v", err)
	}
	if updated.DisplayName != "Updated Name" {
		t.Errorf("display_name = %q; want %q", updated.DisplayName, "Updated Name")
	}
}

func TestStore_List(t *testing.T) {
	t.Parallel()
	pool := testhelper.NewTestPool(t)
	s := principals.NewStore(pool)
	ctx := context.Background()

	_, err := s.Create(ctx, "user", "store-list-user-a", "User A", nil)
	if err != nil {
		t.Fatalf("Create A: %v", err)
	}
	_, err = s.Create(ctx, "user", "store-list-user-b", "User B", nil)
	if err != nil {
		t.Fatalf("Create B: %v", err)
	}

	list, err := s.List(ctx, 10, 0)
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(list) < 2 {
		t.Errorf("expected at least 2 results, got %d", len(list))
	}
}

func TestStore_Delete(t *testing.T) {
	t.Parallel()
	pool := testhelper.NewTestPool(t)
	s := principals.NewStore(pool)
	ctx := context.Background()

	created, err := s.Create(ctx, "agent", "store-delete-bot", "Bot", nil)
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	if err := s.Delete(ctx, created.ID); err != nil {
		t.Fatalf("Delete: %v", err)
	}

	got, err := s.GetByID(ctx, created.ID)
	if err != nil {
		t.Fatalf("GetByID after delete: %v", err)
	}
	if got != nil {
		t.Error("expected nil after delete, got a record")
	}
}
