//go:build integration

package knowledge_test

import (
	"context"
	"testing"

	"github.com/simplyblock/postbrain/internal/knowledge"
	"github.com/simplyblock/postbrain/internal/testhelper"
)

func TestCollectionStore_Create_GetByID(t *testing.T) {
	t.Parallel()
	pool := testhelper.NewTestPool(t)
	cs := knowledge.NewCollectionStore(pool)
	ctx := context.Background()

	principal := testhelper.CreateTestPrincipal(t, pool, "user", "coll-create-user")
	scope := testhelper.CreateTestScope(t, pool, "project", "coll-create-scope", nil, principal.ID)

	desc := "a test collection"
	coll, err := cs.Create(ctx, scope.ID, principal.ID, "coll-create-slug", "Create Test", "project", &desc)
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if coll.ID.String() == "" {
		t.Fatal("expected non-zero collection ID")
	}

	got, err := cs.GetByID(ctx, coll.ID)
	if err != nil {
		t.Fatalf("GetByID: %v", err)
	}
	if got == nil {
		t.Fatal("GetByID returned nil")
	}
	if got.ID != coll.ID {
		t.Errorf("id mismatch: got %v want %v", got.ID, coll.ID)
	}
	if got.Name != "Create Test" {
		t.Errorf("name = %q; want %q", got.Name, "Create Test")
	}
}

func TestCollectionStore_GetBySlug(t *testing.T) {
	t.Parallel()
	pool := testhelper.NewTestPool(t)
	cs := knowledge.NewCollectionStore(pool)
	ctx := context.Background()

	principal := testhelper.CreateTestPrincipal(t, pool, "user", "coll-slug-user")
	scope := testhelper.CreateTestScope(t, pool, "project", "coll-slug-scope", nil, principal.ID)

	coll, err := cs.Create(ctx, scope.ID, principal.ID, "coll-slug-unique", "Slug Test", "private", nil)
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	got, err := cs.GetBySlug(ctx, scope.ID, "coll-slug-unique")
	if err != nil {
		t.Fatalf("GetBySlug: %v", err)
	}
	if got == nil {
		t.Fatal("GetBySlug returned nil")
	}
	if got.ID != coll.ID {
		t.Errorf("id mismatch: got %v want %v", got.ID, coll.ID)
	}
}

func TestCollectionStore_List(t *testing.T) {
	t.Parallel()
	pool := testhelper.NewTestPool(t)
	cs := knowledge.NewCollectionStore(pool)
	ctx := context.Background()

	principal := testhelper.CreateTestPrincipal(t, pool, "user", "coll-list-user")
	scope := testhelper.CreateTestScope(t, pool, "project", "coll-list-scope", nil, principal.ID)

	for i, slug := range []string{"coll-list-a", "coll-list-b"} {
		if _, err := cs.Create(ctx, scope.ID, principal.ID, slug, slug, "project", nil); err != nil {
			t.Fatalf("Create %d: %v", i, err)
		}
	}

	list, err := cs.List(ctx, scope.ID)
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(list) < 2 {
		t.Errorf("expected at least 2 collections, got %d", len(list))
	}
}

func TestCollectionStore_AddItem_ListItems(t *testing.T) {
	t.Parallel()
	pool := testhelper.NewTestPool(t)
	cs := knowledge.NewCollectionStore(pool)
	ctx := context.Background()

	principal := testhelper.CreateTestPrincipal(t, pool, "user", "coll-additem-user")
	scope := testhelper.CreateTestScope(t, pool, "project", "coll-additem-scope", nil, principal.ID)
	testhelper.CreateTestEmbeddingModel(t, pool)
	artifact := testhelper.CreateTestArtifact(t, pool, scope.ID, principal.ID, "collection item artifact")

	coll, err := cs.Create(ctx, scope.ID, principal.ID, "coll-additem-slug", "AddItem Test", "project", nil)
	if err != nil {
		t.Fatalf("Create collection: %v", err)
	}

	if err := cs.AddItem(ctx, coll.ID, artifact.ID, principal.ID); err != nil {
		t.Fatalf("AddItem: %v", err)
	}

	items, err := cs.ListItems(ctx, coll.ID)
	if err != nil {
		t.Fatalf("ListItems: %v", err)
	}
	if len(items) != 1 {
		t.Fatalf("expected 1 item, got %d", len(items))
	}
	if items[0].ID != artifact.ID {
		t.Errorf("item id = %v; want %v", items[0].ID, artifact.ID)
	}
}

func TestCollectionStore_RemoveItem(t *testing.T) {
	t.Parallel()
	pool := testhelper.NewTestPool(t)
	cs := knowledge.NewCollectionStore(pool)
	ctx := context.Background()

	principal := testhelper.CreateTestPrincipal(t, pool, "user", "coll-removeitem-user")
	scope := testhelper.CreateTestScope(t, pool, "project", "coll-removeitem-scope", nil, principal.ID)
	testhelper.CreateTestEmbeddingModel(t, pool)
	artifact := testhelper.CreateTestArtifact(t, pool, scope.ID, principal.ID, "remove item artifact")

	coll, err := cs.Create(ctx, scope.ID, principal.ID, "coll-removeitem-slug", "RemoveItem Test", "project", nil)
	if err != nil {
		t.Fatalf("Create collection: %v", err)
	}

	if err := cs.AddItem(ctx, coll.ID, artifact.ID, principal.ID); err != nil {
		t.Fatalf("AddItem: %v", err)
	}
	if err := cs.RemoveItem(ctx, coll.ID, artifact.ID); err != nil {
		t.Fatalf("RemoveItem: %v", err)
	}

	items, err := cs.ListItems(ctx, coll.ID)
	if err != nil {
		t.Fatalf("ListItems after RemoveItem: %v", err)
	}
	for _, it := range items {
		if it.ID == artifact.ID {
			t.Error("artifact still present after RemoveItem")
		}
	}
}
