//go:build integration

package skills_test

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/simplyblock/postbrain/internal/db"
	"github.com/simplyblock/postbrain/internal/skills"
	"github.com/simplyblock/postbrain/internal/testhelper"
)

// publishSkill transitions a skill through draft → in_review → published.
// It creates a fresh endorser principal for each call.
func publishSkill(t *testing.T, pool *pgxpool.Pool, skillID, authorID uuid.UUID) {
	t.Helper()
	ctx := context.Background()
	lc := skills.NewLifecycle(pool, nil)
	if err := lc.SubmitForReview(ctx, skillID, authorID); err != nil {
		t.Fatalf("SubmitForReview: %v", err)
	}
	endorser := testhelper.CreateTestPrincipal(t, pool, "user", "endorser-"+uuid.New().String())
	if _, err := lc.Endorse(ctx, skillID, endorser.ID, nil); err != nil {
		t.Fatalf("Endorse: %v", err)
	}
}

func TestSkillInstall_WritesFile(t *testing.T) {
	pool := testhelper.NewTestPool(t)
	testhelper.CreateTestEmbeddingModel(t, pool)
	svc := testhelper.NewMockEmbeddingService()
	ctx := t.Context()

	author := testhelper.CreateTestPrincipal(t, pool, "user", "skill-author")
	scope := testhelper.CreateTestScope(t, pool, "project", "acme_skills", nil, author.ID)

	store := skills.NewStore(pool, svc)
	params := []db.SkillParameter{
		{Name: "pr_number", Type: "integer", Required: true, Description: "PR number"},
	}

	skill, err := store.Create(ctx, skills.CreateInput{
		ScopeID:     scope.ID,
		AuthorID:    author.ID,
		Slug:        "review-pr",
		Name:        "Review Pull Request",
		Description: "Review a pull request for issues",
		AgentTypes:  []string{"claude-code"},
		Body:        "Review PR #$PR_NUMBER for issues.",
		Parameters:  params,
		Visibility:  "team",
	})
	if err != nil {
		t.Fatalf("Create skill: %v", err)
	}

	workdir := t.TempDir()
	path, err := skills.Install(skill, nil, "claude-code", workdir)
	if err != nil {
		t.Fatalf("Install: %v", err)
	}
	if path == "" {
		t.Error("expected non-empty path")
	}

	if !skills.IsInstalled("review-pr", "claude-code", workdir) {
		t.Error("IsInstalled: expected true after install")
	}
}

func TestSkillInvoke_ValidParams(t *testing.T) {
	pool := testhelper.NewTestPool(t)
	testhelper.CreateTestEmbeddingModel(t, pool)
	svc := testhelper.NewMockEmbeddingService()
	ctx := t.Context()

	author := testhelper.CreateTestPrincipal(t, pool, "user", "invoke-author")
	scope := testhelper.CreateTestScope(t, pool, "project", "acme_invoke", nil, author.ID)

	store := skills.NewStore(pool, svc)
	skill, err := store.Create(ctx, skills.CreateInput{
		ScopeID:     scope.ID,
		AuthorID:    author.ID,
		Slug:        "greet",
		Name:        "Greet",
		Description: "Greet a user",
		// The invoke logic substitutes $NAME and $AGE (uppercase) or {{name}}/{{age}}.
		Body: "Hello, $NAME! You are $AGE years old.",
		Parameters: []db.SkillParameter{
			{Name: "name", Type: "string", Required: true},
			{Name: "age", Type: "integer", Required: true},
		},
		Visibility: "team",
	})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	body, err := skills.Invoke(skill, map[string]any{"name": "Alice", "age": float64(30)})
	if err != nil {
		t.Fatalf("Invoke: %v", err)
	}
	if body != "Hello, Alice! You are 30 years old." {
		t.Errorf("unexpected body: %q", body)
	}
}

func TestSkillInvoke_MissingRequiredParam(t *testing.T) {
	paramsJSON, _ := json.Marshal([]db.SkillParameter{
		{Name: "name", Type: "string", Required: true},
	})
	skill := &db.Skill{
		Slug:       "test",
		Body:       "Hello $NAME",
		Parameters: paramsJSON,
	}
	_, err := skills.Invoke(skill, map[string]any{})
	if err == nil {
		t.Error("expected ValidationError")
	}
	var ve *skills.ValidationError
	if !errors.As(err, &ve) {
		t.Errorf("expected *ValidationError, got %T", err)
	}
}

// ── Store: GetByID / GetBySlug / Update ───────────────────────────────────────

func TestStore_GetByID(t *testing.T) {
	pool := testhelper.NewTestPool(t)
	testhelper.CreateTestEmbeddingModel(t, pool)
	svc := testhelper.NewMockEmbeddingService()
	ctx := t.Context()

	author := testhelper.CreateTestPrincipal(t, pool, "user", "getbyid-author")
	scope := testhelper.CreateTestScope(t, pool, "project", "getbyid-scope", nil, author.ID)
	store := skills.NewStore(pool, svc)

	skill, err := store.Create(ctx, skills.CreateInput{
		ScopeID: scope.ID, AuthorID: author.ID, Slug: "getbyid-skill",
		Name: "GetByID Skill", Description: "desc", Body: "body", Visibility: "team",
	})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	got, err := store.GetByID(ctx, skill.ID)
	if err != nil {
		t.Fatalf("GetByID: %v", err)
	}
	if got == nil || got.ID != skill.ID {
		t.Errorf("GetByID returned wrong record: %+v", got)
	}

	// Unknown ID → nil.
	missing, err := store.GetByID(ctx, uuid.New())
	if err != nil {
		t.Fatalf("GetByID unknown: %v", err)
	}
	if missing != nil {
		t.Error("expected nil for unknown ID")
	}
}

func TestStore_GetBySlug(t *testing.T) {
	pool := testhelper.NewTestPool(t)
	testhelper.CreateTestEmbeddingModel(t, pool)
	svc := testhelper.NewMockEmbeddingService()
	ctx := t.Context()

	author := testhelper.CreateTestPrincipal(t, pool, "user", "getbyslug-author")
	scope := testhelper.CreateTestScope(t, pool, "project", "getbyslug-scope", nil, author.ID)
	store := skills.NewStore(pool, svc)

	skill, err := store.Create(ctx, skills.CreateInput{
		ScopeID: scope.ID, AuthorID: author.ID, Slug: "getbyslug-skill",
		Name: "GetBySlug Skill", Description: "desc", Body: "body", Visibility: "team",
	})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	got, err := store.GetBySlug(ctx, scope.ID, "getbyslug-skill")
	if err != nil {
		t.Fatalf("GetBySlug: %v", err)
	}
	if got == nil || got.ID != skill.ID {
		t.Errorf("GetBySlug returned wrong record: %+v", got)
	}

	// Unknown slug → nil.
	missing, err := store.GetBySlug(ctx, scope.ID, "no-such-slug")
	if err != nil {
		t.Fatalf("GetBySlug unknown: %v", err)
	}
	if missing != nil {
		t.Error("expected nil for unknown slug")
	}
}

func TestStore_Update(t *testing.T) {
	pool := testhelper.NewTestPool(t)
	testhelper.CreateTestEmbeddingModel(t, pool)
	svc := testhelper.NewMockEmbeddingService()
	ctx := t.Context()

	author := testhelper.CreateTestPrincipal(t, pool, "user", "update-author")
	scope := testhelper.CreateTestScope(t, pool, "project", "update-scope", nil, author.ID)
	store := skills.NewStore(pool, svc)

	skill, err := store.Create(ctx, skills.CreateInput{
		ScopeID: scope.ID, AuthorID: author.ID, Slug: "update-skill",
		Name: "Update Skill", Description: "original desc", Body: "original body", Visibility: "team",
	})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	updated, err := store.Update(ctx, skill.ID, author.ID, "updated body", []db.SkillParameter{})
	if err != nil {
		t.Fatalf("Update: %v", err)
	}
	if updated.Body != "updated body" {
		t.Errorf("body = %q; want %q", updated.Body, "updated body")
	}
	if updated.Version <= skill.Version {
		t.Errorf("expected version to increment; before=%d after=%d", skill.Version, updated.Version)
	}
}

// ── Recall ────────────────────────────────────────────────────────────────────

func TestRecall_EmptyDB_ReturnsEmpty(t *testing.T) {
	pool := testhelper.NewTestPool(t)
	testhelper.CreateTestEmbeddingModel(t, pool)
	svc := testhelper.NewMockEmbeddingService()
	ctx := t.Context()

	author := testhelper.CreateTestPrincipal(t, pool, "user", "recall-empty-author")
	scope := testhelper.CreateTestScope(t, pool, "project", "recall-empty-scope", nil, author.ID)
	store := skills.NewStore(pool, svc)

	results, err := store.Recall(ctx, svc, skills.RecallInput{
		Query: "anything", ScopeIDs: []uuid.UUID{scope.ID}, Limit: 10,
	})
	if err != nil {
		t.Fatalf("Recall: %v", err)
	}
	if len(results) != 0 {
		t.Errorf("expected 0 results on empty DB, got %d", len(results))
	}
}

func TestRecall_PublishedSkillFound(t *testing.T) {
	pool := testhelper.NewTestPool(t)
	testhelper.CreateTestEmbeddingModel(t, pool)
	svc := testhelper.NewMockEmbeddingService()
	ctx := t.Context()

	author := testhelper.CreateTestPrincipal(t, pool, "user", "recall-found-author")
	scope := testhelper.CreateTestScope(t, pool, "project", "recall-found-scope", nil, author.ID)
	store := skills.NewStore(pool, svc)

	skill, err := store.Create(ctx, skills.CreateInput{
		ScopeID: scope.ID, AuthorID: author.ID, Slug: "recall-found-skill",
		Name: "Recall Found Skill", Description: "recall integration test", Body: "recall body",
		Visibility: "team", ReviewRequired: 1,
	})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	publishSkill(t, pool, skill.ID, author.ID)

	results, err := store.Recall(ctx, svc, skills.RecallInput{
		Query: skill.Description, ScopeIDs: []uuid.UUID{scope.ID}, Limit: 10,
	})
	if err != nil {
		t.Fatalf("Recall: %v", err)
	}
	found := false
	for _, r := range results {
		if r.Skill.ID == skill.ID {
			found = true
		}
	}
	if !found {
		t.Errorf("expected published skill %v in Recall results, got %d results", skill.ID, len(results))
	}
}

func TestRecall_UsesModelTableWhenLegacyEmbeddingMissing(t *testing.T) {
	pool := testhelper.NewTestPool(t)
	svc := testhelper.NewMockEmbeddingService()
	ctx := t.Context()

	author := testhelper.CreateTestPrincipal(t, pool, "user", "recall-modeltable-author")
	scope := testhelper.CreateTestScope(t, pool, "project", "recall-modeltable-scope", nil, author.ID)
	store := skills.NewStore(pool, svc)

	model, err := db.RegisterEmbeddingModel(ctx, pool, db.RegisterEmbeddingModelParams{
		Slug:          "skills-recall-modeltable-" + uuid.NewString(),
		Provider:      "ollama",
		ServiceURL:    "http://localhost:11434",
		ProviderModel: "nomic-embed-text",
		Dimensions:    4,
		ContentType:   "text",
		Activate:      true,
	})
	if err != nil {
		t.Fatalf("register model: %v", err)
	}

	skill, err := store.Create(ctx, skills.CreateInput{
		ScopeID:     scope.ID,
		AuthorID:    author.ID,
		Slug:        "modeltable-skill",
		Name:        "ModelTable Skill",
		Description: "model table recall integration",
		Body:        "body",
		Visibility:  "team",
	})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	publishSkill(t, pool, skill.ID, author.ID)

	query := "model table recall integration"
	queryVec, err := svc.EmbedText(ctx, query)
	if err != nil {
		t.Fatalf("EmbedText: %v", err)
	}
	repo := db.NewEmbeddingRepository(pool)
	if err := repo.UpsertEmbedding(ctx, db.UpsertEmbeddingInput{
		ObjectType: "skill",
		ObjectID:   skill.ID,
		ScopeID:    scope.ID,
		ModelID:    model.ID,
		Embedding:  queryVec,
	}); err != nil {
		t.Fatalf("seed model-table embedding: %v", err)
	}
	if _, err := pool.Exec(ctx, `
		UPDATE skills
		SET embedding = NULL, embedding_model_id = NULL
		WHERE id = $1
	`, skill.ID); err != nil {
		t.Fatalf("clear legacy embedding columns: %v", err)
	}

	results, err := store.Recall(ctx, svc, skills.RecallInput{
		Query:     query,
		ScopeIDs:  []uuid.UUID{scope.ID},
		AgentType: "any",
		Limit:     10,
	})
	if err != nil {
		t.Fatalf("Recall: %v", err)
	}
	found := false
	for _, r := range results {
		if r.Skill.ID == skill.ID {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("expected skill %s in results", skill.ID)
	}
}

func TestRecall_LimitRespected(t *testing.T) {
	pool := testhelper.NewTestPool(t)
	testhelper.CreateTestEmbeddingModel(t, pool)
	svc := testhelper.NewMockEmbeddingService()
	ctx := t.Context()

	author := testhelper.CreateTestPrincipal(t, pool, "user", "recall-limit-author")
	scope := testhelper.CreateTestScope(t, pool, "project", "recall-limit-scope", nil, author.ID)
	store := skills.NewStore(pool, svc)

	// Publish 3 skills.
	for i, slug := range []string{"limit-skill-a", "limit-skill-b", "limit-skill-c"} {
		sk, err := store.Create(ctx, skills.CreateInput{
			ScopeID: scope.ID, AuthorID: author.ID, Slug: slug,
			Name: slug, Description: "limit test skill", Body: "body", Visibility: "team",
			ReviewRequired: 1,
		})
		if err != nil {
			t.Fatalf("Create %d: %v", i, err)
		}
		publishSkill(t, pool, sk.ID, author.ID)
	}

	results, err := store.Recall(ctx, svc, skills.RecallInput{
		Query: "limit test skill", ScopeIDs: []uuid.UUID{scope.ID}, Limit: 2,
	})
	if err != nil {
		t.Fatalf("Recall: %v", err)
	}
	if len(results) > 2 {
		t.Errorf("expected at most 2 results with Limit=2, got %d", len(results))
	}
}

func TestRecall_InstalledFilter(t *testing.T) {
	pool := testhelper.NewTestPool(t)
	testhelper.CreateTestEmbeddingModel(t, pool)
	svc := testhelper.NewMockEmbeddingService()
	ctx := t.Context()
	workdir := t.TempDir()

	author := testhelper.CreateTestPrincipal(t, pool, "user", "recall-installed-author")
	scope := testhelper.CreateTestScope(t, pool, "project", "recall-installed-scope", nil, author.ID)
	store := skills.NewStore(pool, svc)

	// Create and publish two skills; install only the first.
	create := func(slug string) *db.Skill {
		sk, err := store.Create(ctx, skills.CreateInput{
			ScopeID: scope.ID, AuthorID: author.ID, Slug: slug,
			Name: slug, Description: "installed filter test", Body: "body",
			Visibility: "team", ReviewRequired: 1,
		})
		if err != nil {
			t.Fatalf("Create %s: %v", slug, err)
		}
		publishSkill(t, pool, sk.ID, author.ID)
		return sk
	}
	installed := create("installed-skill")
	_ = create("not-installed-skill")

	if _, err := skills.Install(installed, nil, "any", workdir); err != nil {
		t.Fatalf("Install: %v", err)
	}

	want := true
	results, err := store.Recall(ctx, svc, skills.RecallInput{
		Query: "installed filter test", ScopeIDs: []uuid.UUID{scope.ID},
		Limit: 10, Workdir: workdir, Installed: &want,
	})
	if err != nil {
		t.Fatalf("Recall Installed=true: %v", err)
	}
	for _, r := range results {
		if !r.Installed {
			t.Errorf("Installed=true filter returned non-installed skill %v", r.Skill.ID)
		}
	}
	if len(results) == 0 {
		t.Error("expected at least one installed skill in results")
	}
}
