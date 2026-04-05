//go:build integration

package knowledge_test

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/simplyblock/postbrain/internal/config"
	"github.com/simplyblock/postbrain/internal/db"
	"github.com/simplyblock/postbrain/internal/embedding"
	"github.com/simplyblock/postbrain/internal/knowledge"
	"github.com/simplyblock/postbrain/internal/testhelper"
)

func TestKnowledgeCreate_DualWritesToEmbeddingRepository(t *testing.T) {
	pool := testhelper.NewTestPool(t)
	ctx := context.Background()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/embeddings" {
			http.NotFound(w, r)
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"data": []map[string]any{{
				"index":     0,
				"embedding": []float32{0.2, 0.3, 0.4, 0.5},
			}},
		})
	}))
	defer server.Close()

	model, err := db.RegisterEmbeddingModel(ctx, pool, db.RegisterEmbeddingModelParams{
		Slug:          "knw-dual-" + uuid.NewString(),
		Provider:      "openai",
		ServiceURL:    server.URL + "/v1",
		ProviderModel: "text-embedding-3-small",
		Dimensions:    4,
		ContentType:   "text",
		Activate:      true,
	})
	if err != nil {
		t.Fatalf("register model: %v", err)
	}

	cfg := &config.EmbeddingConfig{
		RequestTimeout: 5 * time.Second,
		BatchSize:      8,
		Providers: map[string]config.EmbeddingProviderConfig{
			"default": {
				Backend:    "openai",
				ServiceURL: server.URL + "/v1",
				TextModel:  "unused-static-model",
			},
		},
	}
	svc, err := embedding.NewService(cfg)
	if err != nil {
		t.Fatalf("NewService: %v", err)
	}
	if err := svc.EnableModelDrivenFactory(ctx, pool, cfg); err != nil {
		t.Fatalf("EnableModelDrivenFactory: %v", err)
	}

	author := testhelper.CreateTestPrincipal(t, pool, "user", "knw-dual-user")
	scope := testhelper.CreateTestScope(t, pool, "project", "knw/dual", nil, author.ID)
	store := knowledge.NewStore(pool, svc)

	artifact, err := store.Create(ctx, knowledge.CreateInput{
		KnowledgeType: "semantic",
		OwnerScopeID:  scope.ID,
		AuthorID:      author.ID,
		Visibility:    "project",
		Title:         "Dual write artifact",
		Content:       "Dual write content",
	})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	var status string
	err = pool.QueryRow(ctx, `
		SELECT status FROM embedding_index
		WHERE object_type = 'knowledge_artifact' AND object_id = $1 AND model_id = $2
	`, artifact.ID, model.ID).Scan(&status)
	if err != nil {
		t.Fatalf("select embedding_index row: %v", err)
	}
	if status != "ready" {
		t.Fatalf("embedding_index status = %q, want ready", status)
	}

	var tableName string
	err = pool.QueryRow(ctx, `SELECT table_name FROM embedding_models WHERE id=$1`, model.ID).Scan(&tableName)
	if err != nil {
		t.Fatalf("select model table name: %v", err)
	}
	var exists bool
	err = pool.QueryRow(ctx, fmt.Sprintf(`
		SELECT EXISTS (
			SELECT 1 FROM %s WHERE object_type='knowledge_artifact' AND object_id=$1
		)
	`, tableName), artifact.ID).Scan(&exists)
	if err != nil {
		t.Fatalf("select model table row: %v", err)
	}
	if !exists {
		t.Fatal("expected dual-write row in model table")
	}
}
