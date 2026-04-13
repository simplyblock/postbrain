//go:build integration

package skills_test

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
	"github.com/simplyblock/postbrain/internal/skills"
	"github.com/simplyblock/postbrain/internal/modelruntime"
	"github.com/simplyblock/postbrain/internal/testhelper"
)

func TestSkillsCreate_DualWritesToEmbeddingRepository(t *testing.T) {
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
				"embedding": []float32{0.6, 0.7, 0.8, 0.9},
			}},
		})
	}))
	defer server.Close()

	model, err := db.RegisterEmbeddingModel(ctx, pool, db.RegisterEmbeddingModelParams{
		Slug:          "skill-dual-" + uuid.NewString(),
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
	if err := modelruntime.EnableModelDrivenFactory(ctx, svc, pool, cfg); err != nil {
		t.Fatalf("EnableModelDrivenFactory: %v", err)
	}

	author := testhelper.CreateTestPrincipal(t, pool, "user", "skill-dual-user")
	scope := testhelper.CreateTestScope(t, pool, "project", "skill/dual", nil, author.ID)
	store := skills.NewStore(pool, svc)

	skill, err := store.Create(ctx, skills.CreateInput{
		ScopeID:        scope.ID,
		AuthorID:       author.ID,
		Slug:           "dual-skill",
		Name:           "Dual Skill",
		Description:    "Dual write skill",
		Body:           "Use this skill",
		Visibility:     "project",
		ReviewRequired: 1,
	})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	var status string
	err = pool.QueryRow(ctx, `
		SELECT status FROM embedding_index
		WHERE object_type = 'skill' AND object_id = $1 AND model_id = $2
	`, skill.ID, model.ID).Scan(&status)
	if err != nil {
		t.Fatalf("select embedding_index row: %v", err)
	}
	if status != "ready" {
		t.Fatalf("embedding_index status = %q, want ready", status)
	}

	var tableName string
	err = pool.QueryRow(ctx, `SELECT table_name FROM ai_models WHERE id=$1`, model.ID).Scan(&tableName)
	if err != nil {
		t.Fatalf("select model table name: %v", err)
	}
	var exists bool
	err = pool.QueryRow(ctx, fmt.Sprintf(`
		SELECT EXISTS (
			SELECT 1 FROM %s WHERE object_type='skill' AND object_id=$1
		)
	`, tableName), skill.ID).Scan(&exists)
	if err != nil {
		t.Fatalf("select model table row: %v", err)
	}
	if !exists {
		t.Fatal("expected dual-write row in model table")
	}
}
