//go:build integration

package jobs

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"

	"github.com/simplyblock/postbrain/internal/db"
	"github.com/simplyblock/postbrain/internal/providers"
	"github.com/simplyblock/postbrain/internal/testhelper"
)

type failingEmbedder struct{}

func (f *failingEmbedder) Embed(_ context.Context, _ string) ([]float32, error) {
	return nil, errors.New("forced embed error")
}
func (f *failingEmbedder) EmbedBatch(_ context.Context, _ []string) ([][]float32, error) {
	return nil, errors.New("forced embed error")
}
func (f *failingEmbedder) Dimensions() int {
	return 4
}
func (f *failingEmbedder) ModelSlug() string {
	return "failing"
}

type recordingEmbedder struct {
	lastInput string
}

func (r *recordingEmbedder) Embed(_ context.Context, text string) ([]float32, error) {
	r.lastInput = text
	return []float32{0.1, 0.2, 0.3, 0.4}, nil
}
func (r *recordingEmbedder) EmbedBatch(_ context.Context, _ []string) ([][]float32, error) {
	return nil, errors.New("not implemented")
}
func (r *recordingEmbedder) Dimensions() int { return 4 }
func (r *recordingEmbedder) ModelSlug() string {
	return "recording"
}

func TestReembedJob_RunText_NoActiveModel(t *testing.T) {
	t.Parallel()
	pool := testhelper.NewTestPool(t)
	svc := testhelper.NewMockEmbeddingService()
	ctx := context.Background()

	// No embedding_model is inserted → activeModelID returns nil → RunText returns nil.
	j := NewReembedJob(pool, svc, 1)
	if err := j.RunText(ctx); err != nil {
		t.Fatalf("RunText with no active model: %v", err)
	}
}

func TestReembedJob_RunText_ReembedsMismatchedMemory(t *testing.T) {
	t.Parallel()
	pool := testhelper.NewTestPool(t)
	svc := testhelper.NewMockEmbeddingService()
	ctx := context.Background()

	// Insert active text embedding model.
	activeID := testhelper.CreateTestEmbeddingModel(t, pool)

	// Create a memory whose embedding_model_id is NULL (mismatched).
	principal := testhelper.CreateTestPrincipal(t, pool, "user", "reembed-text-user")
	scope := testhelper.CreateTestScope(t, pool, "project", "reembed-text-scope", nil, principal.ID)
	mem := testhelper.CreateTestMemory(t, pool, scope.ID, principal.ID, "reembed text content")
	if _, err := pool.Exec(ctx, `
		INSERT INTO embedding_index (object_type, object_id, model_id, status, retry_count)
		VALUES ('memory', $1, $2, 'pending', 0)
		ON CONFLICT (object_type, object_id, model_id)
		DO UPDATE SET status='pending', retry_count=0, last_error=NULL
	`, mem.ID, activeID); err != nil {
		t.Fatalf("seed embedding_index pending row: %v", err)
	}

	// Verify embedding_model_id starts NULL.
	var beforeModelID *uuid.UUID
	if err := pool.QueryRow(ctx,
		`SELECT embedding_model_id FROM memories WHERE id = $1`, mem.ID,
	).Scan(&beforeModelID); err != nil {
		t.Fatalf("scan before RunText: %v", err)
	}
	if beforeModelID != nil {
		t.Fatalf("expected embedding_model_id=NULL before reembed, got %v", beforeModelID)
	}

	j := NewReembedJob(pool, svc, 64)
	if err := j.RunText(ctx); err != nil {
		t.Fatalf("RunText: %v", err)
	}

	var afterModelID *uuid.UUID
	if err := pool.QueryRow(ctx,
		`SELECT embedding_model_id FROM memories WHERE id = $1`, mem.ID,
	).Scan(&afterModelID); err != nil {
		t.Fatalf("scan after RunText: %v", err)
	}
	if afterModelID == nil {
		t.Fatal("expected embedding_model_id to be set after RunText")
	}
	if *afterModelID != activeID {
		t.Errorf("embedding_model_id = %v; want %v", *afterModelID, activeID)
	}
}

func TestReembedJob_RunCode_ReembedsMismatchedCodeMemory(t *testing.T) {
	t.Parallel()
	pool := testhelper.NewTestPool(t)
	svc := testhelper.NewMockEmbeddingService()
	ctx := context.Background()

	// Insert active code embedding model.
	var codeModelID uuid.UUID
	if err := pool.QueryRow(ctx,
		`INSERT INTO ai_models (slug, dimensions, content_type, model_type, is_active)
		 VALUES ('test/code-model', 4, 'code', 'embedding', true)
		 RETURNING id`,
	).Scan(&codeModelID); err != nil {
		t.Fatalf("create code embedding model: %v", err)
	}

	// Create a memory with content_kind='code' and no code embedding model set.
	principal := testhelper.CreateTestPrincipal(t, pool, "user", "reembed-code-user")
	scope := testhelper.CreateTestScope(t, pool, "project", "reembed-code-scope", nil, principal.ID)

	var memID uuid.UUID
	if err := pool.QueryRow(ctx,
		`INSERT INTO memories (memory_type, scope_id, author_id, content, content_kind, is_active)
		 VALUES ('semantic', $1, $2, 'fn main() {}', 'code', true)
		 RETURNING id`,
		scope.ID, principal.ID,
	).Scan(&memID); err != nil {
		t.Fatalf("insert code memory: %v", err)
	}
	if _, err := pool.Exec(ctx, `
		INSERT INTO embedding_index (object_type, object_id, model_id, status, retry_count)
		VALUES ('memory', $1, $2, 'pending', 0)
		ON CONFLICT (object_type, object_id, model_id)
		DO UPDATE SET status='pending', retry_count=0, last_error=NULL
	`, memID, codeModelID); err != nil {
		t.Fatalf("seed embedding_index pending row: %v", err)
	}

	j := NewReembedJob(pool, svc, 64)
	if err := j.RunCode(ctx); err != nil {
		t.Fatalf("RunCode: %v", err)
	}

	var afterModelID *uuid.UUID
	if err := pool.QueryRow(ctx,
		`SELECT embedding_code_model_id FROM memories WHERE id = $1`, memID,
	).Scan(&afterModelID); err != nil {
		t.Fatalf("scan after RunCode: %v", err)
	}
	if afterModelID == nil {
		t.Fatal("expected embedding_code_model_id to be set after RunCode")
	}
	if *afterModelID != codeModelID {
		t.Errorf("embedding_code_model_id = %v; want %v", *afterModelID, codeModelID)
	}
}

func TestReembedJob_RunText_UsesEmbeddingIndexPendingAndMarksReady(t *testing.T) {
	t.Parallel()
	pool := testhelper.NewTestPool(t)
	svc := testhelper.NewMockEmbeddingService()
	ctx := context.Background()

	model, err := db.RegisterEmbeddingModel(ctx, pool, db.RegisterEmbeddingModelParams{
		Slug:          "reembed-text-ready-" + uuid.NewString(),
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
	modelID := model.ID

	principal := testhelper.CreateTestPrincipal(t, pool, "user", "reembed-index-user")
	scope := testhelper.CreateTestScope(t, pool, "project", "reembed-index-scope", nil, principal.ID)
	mem := testhelper.CreateTestMemory(t, pool, scope.ID, principal.ID, "reembed index content")

	if _, err := pool.Exec(ctx, `
		UPDATE memories SET embedding = NULL, embedding_model_id = NULL WHERE id = $1
	`, mem.ID); err != nil {
		t.Fatalf("clear legacy embedding: %v", err)
	}
	if _, err := pool.Exec(ctx, `
		INSERT INTO embedding_index (object_type, object_id, model_id, status, retry_count)
		VALUES ('memory', $1, $2, 'pending', 0)
		ON CONFLICT (object_type, object_id, model_id)
		DO UPDATE SET status='pending', retry_count=0, last_error=NULL
	`, mem.ID, modelID); err != nil {
		t.Fatalf("seed embedding_index pending row: %v", err)
	}

	j := NewReembedJob(pool, svc, 64)
	if err := j.RunText(ctx); err != nil {
		t.Fatalf("RunText: %v", err)
	}

	var (
		afterModelID *uuid.UUID
		status       string
		retryCount   int
		lastError    *string
	)
	if err := pool.QueryRow(ctx, `
		SELECT embedding_model_id FROM memories WHERE id = $1
	`, mem.ID).Scan(&afterModelID); err != nil {
		t.Fatalf("scan memory after RunText: %v", err)
	}
	if afterModelID == nil || *afterModelID != modelID {
		t.Fatalf("embedding_model_id = %v, want %v", afterModelID, modelID)
	}
	if err := pool.QueryRow(ctx, `
		SELECT status, retry_count, last_error
		FROM embedding_index
		WHERE object_type='memory' AND object_id=$1 AND model_id=$2
	`, mem.ID, modelID).Scan(&status, &retryCount, &lastError); err != nil {
		t.Fatalf("scan embedding_index after RunText: %v", err)
	}
	if status != "ready" {
		t.Fatalf("status = %q, want ready", status)
	}
	if retryCount != 0 {
		t.Fatalf("retry_count = %d, want 0", retryCount)
	}
	if lastError != nil {
		t.Fatalf("last_error = %v, want NULL", *lastError)
	}
}

func TestReembedJob_RunText_FailureIncrementsRetryAndEventuallyFailed(t *testing.T) {
	t.Parallel()
	pool := testhelper.NewTestPool(t)
	svc := providers.NewServiceFromEmbedders(&failingEmbedder{}, nil)
	ctx := context.Background()

	model, err := db.RegisterEmbeddingModel(ctx, pool, db.RegisterEmbeddingModelParams{
		Slug:          "reembed-text-fail-" + uuid.NewString(),
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
	modelID := model.ID

	principal := testhelper.CreateTestPrincipal(t, pool, "user", "reembed-fail-user")
	scope := testhelper.CreateTestScope(t, pool, "project", "reembed-fail-scope", nil, principal.ID)
	mem := testhelper.CreateTestMemory(t, pool, scope.ID, principal.ID, "forced error content")

	if _, err := pool.Exec(ctx, `
		INSERT INTO embedding_index (object_type, object_id, model_id, status, retry_count)
		VALUES ('memory', $1, $2, 'pending', 2)
		ON CONFLICT (object_type, object_id, model_id)
		DO UPDATE SET status='pending', retry_count=2, last_error=NULL
	`, mem.ID, modelID); err != nil {
		t.Fatalf("seed embedding_index pending row: %v", err)
	}

	j := NewReembedJob(pool, svc, 64)
	if err := j.RunText(ctx); err != nil {
		t.Fatalf("RunText: %v", err)
	}

	var (
		status     string
		retryCount int
		lastError  *string
	)
	if err := pool.QueryRow(ctx, `
		SELECT status, retry_count, last_error
		FROM embedding_index
		WHERE object_type='memory' AND object_id=$1 AND model_id=$2
	`, mem.ID, modelID).Scan(&status, &retryCount, &lastError); err != nil {
		t.Fatalf("scan embedding_index after RunText: %v", err)
	}
	if status != "failed" {
		t.Fatalf("status = %q, want failed", status)
	}
	if retryCount != 3 {
		t.Fatalf("retry_count = %d, want 3", retryCount)
	}
	if lastError == nil || *lastError == "" {
		t.Fatal("last_error should be populated on failed embedding row")
	}
}

func TestReembedJob_RunText_SkillUsesDescriptionAndBody(t *testing.T) {
	t.Parallel()
	pool := testhelper.NewTestPool(t)
	rec := &recordingEmbedder{}
	svc := providers.NewServiceFromEmbedders(rec, nil)
	ctx := context.Background()

	model, err := db.RegisterEmbeddingModel(ctx, pool, db.RegisterEmbeddingModelParams{
		Slug:          "reembed-text-skill-content-" + uuid.NewString(),
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
	modelID := model.ID

	principal := testhelper.CreateTestPrincipal(t, pool, "user", "reembed-skill-content-user")
	scope := testhelper.CreateTestScope(t, pool, "project", "reembed-skill-content-scope", nil, principal.ID)
	var skillID uuid.UUID
	if err := pool.QueryRow(ctx, `
		INSERT INTO skills (scope_id, author_id, slug, name, description, body)
		VALUES ($1, $2, $3, $4, $5, $6)
		RETURNING id
	`, scope.ID, principal.ID, "reembed-skill-content-"+uuid.NewString(), "Skill", "desc-text", "body-text").Scan(&skillID); err != nil {
		t.Fatalf("insert skill: %v", err)
	}
	if _, err := pool.Exec(ctx, `
		INSERT INTO embedding_index (object_type, object_id, model_id, status, retry_count)
		VALUES ('skill', $1, $2, 'pending', 0)
		ON CONFLICT (object_type, object_id, model_id)
		DO UPDATE SET status='pending', retry_count=0, last_error=NULL
	`, skillID, modelID); err != nil {
		t.Fatalf("seed embedding_index pending row: %v", err)
	}

	j := NewReembedJob(pool, svc, 64)
	if err := j.RunText(ctx); err != nil {
		t.Fatalf("RunText: %v", err)
	}

	if rec.lastInput != "desc-text body-text" {
		t.Fatalf("embedded content = %q, want %q", rec.lastInput, "desc-text body-text")
	}
}

func TestReembedJob_RunText_EmptyContentMarksFailed(t *testing.T) {
	t.Parallel()
	pool := testhelper.NewTestPool(t)
	svc := testhelper.NewMockEmbeddingService()
	ctx := context.Background()

	model, err := db.RegisterEmbeddingModel(ctx, pool, db.RegisterEmbeddingModelParams{
		Slug:          "reembed-empty-content-" + uuid.NewString(),
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
	modelID := model.ID

	principal := testhelper.CreateTestPrincipal(t, pool, "user", "reembed-empty-content-user")
	scope := testhelper.CreateTestScope(t, pool, "project", "reembed-empty-content-scope", nil, principal.ID)
	var skillID uuid.UUID
	if err := pool.QueryRow(ctx, `
		INSERT INTO skills (scope_id, author_id, slug, name, description, body)
		VALUES ($1, $2, $3, $4, '', '')
		RETURNING id
	`, scope.ID, principal.ID, "reembed-empty-content-"+uuid.NewString(), "Skill").Scan(&skillID); err != nil {
		t.Fatalf("insert skill: %v", err)
	}
	if _, err := pool.Exec(ctx, `
		INSERT INTO embedding_index (object_type, object_id, model_id, status, retry_count)
		VALUES ('skill', $1, $2, 'pending', 2)
		ON CONFLICT (object_type, object_id, model_id)
		DO UPDATE SET status='pending', retry_count=2, last_error=NULL
	`, skillID, modelID); err != nil {
		t.Fatalf("seed embedding_index pending row: %v", err)
	}

	j := NewReembedJob(pool, svc, 64)
	if err := j.RunText(ctx); err != nil {
		t.Fatalf("RunText: %v", err)
	}

	var (
		status     string
		retryCount int
		lastError  *string
	)
	if err := pool.QueryRow(ctx, `
		SELECT status, retry_count, last_error
		FROM embedding_index
		WHERE object_type='skill' AND object_id=$1 AND model_id=$2
	`, skillID, modelID).Scan(&status, &retryCount, &lastError); err != nil {
		t.Fatalf("scan embedding_index after RunText: %v", err)
	}
	if status != "failed" {
		t.Fatalf("status = %q, want failed", status)
	}
	if retryCount != 3 {
		t.Fatalf("retry_count = %d, want 3", retryCount)
	}
	if lastError == nil || *lastError == "" {
		t.Fatal("last_error should be populated on empty-content failure")
	}
}

func TestReembedJob_RunText_NullContentMarksFailed(t *testing.T) {
	t.Parallel()
	pool := testhelper.NewTestPool(t)
	svc := testhelper.NewMockEmbeddingService()
	ctx := context.Background()

	model, err := db.RegisterEmbeddingModel(ctx, pool, db.RegisterEmbeddingModelParams{
		Slug:          "reembed-null-content-" + uuid.NewString(),
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
	modelID := model.ID

	principal := testhelper.CreateTestPrincipal(t, pool, "user", "reembed-null-content-user")
	scope := testhelper.CreateTestScope(t, pool, "project", "reembed-null-content-scope", nil, principal.ID)
	mem := testhelper.CreateTestMemory(t, pool, scope.ID, principal.ID, "null-source")
	if _, err := pool.Exec(ctx, `UPDATE memories SET is_active=false WHERE id=$1`, mem.ID); err != nil {
		t.Fatalf("deactivate memory: %v", err)
	}
	if _, err := pool.Exec(ctx, `
		INSERT INTO embedding_index (object_type, object_id, model_id, status, retry_count)
		VALUES ('memory', $1, $2, 'pending', 2)
		ON CONFLICT (object_type, object_id, model_id)
		DO UPDATE SET status='pending', retry_count=2, last_error=NULL
	`, mem.ID, modelID); err != nil {
		t.Fatalf("seed embedding_index pending row: %v", err)
	}

	j := NewReembedJob(pool, svc, 64)
	if err := j.RunText(ctx); err != nil {
		t.Fatalf("RunText: %v", err)
	}

	var (
		status     string
		retryCount int
		lastError  *string
	)
	if err := pool.QueryRow(ctx, `
		SELECT status, retry_count, last_error
		FROM embedding_index
		WHERE object_type='memory' AND object_id=$1 AND model_id=$2
	`, mem.ID, modelID).Scan(&status, &retryCount, &lastError); err != nil {
		t.Fatalf("scan embedding_index after RunText: %v", err)
	}
	if status != "failed" {
		t.Fatalf("status = %q, want failed", status)
	}
	if retryCount != 3 {
		t.Fatalf("retry_count = %d, want 3", retryCount)
	}
	if lastError == nil || *lastError == "" {
		t.Fatal("last_error should be populated on null-content failure")
	}
}

func TestReembedJob_RunCode_UsesEmbeddingIndexPendingAndMarksReady(t *testing.T) {
	t.Parallel()
	pool := testhelper.NewTestPool(t)
	svc := testhelper.NewMockEmbeddingService()
	ctx := context.Background()

	codeModel, err := db.RegisterEmbeddingModel(ctx, pool, db.RegisterEmbeddingModelParams{
		Slug:          "reembed-code-ready-" + uuid.NewString(),
		Provider:      "ollama",
		ServiceURL:    "http://localhost:11434",
		ProviderModel: "nomic-embed-text",
		Dimensions:    4,
		ContentType:   "code",
		Activate:      true,
	})
	if err != nil {
		t.Fatalf("register code model: %v", err)
	}
	codeModelID := codeModel.ID

	principal := testhelper.CreateTestPrincipal(t, pool, "user", "reembed-code-index-user")
	scope := testhelper.CreateTestScope(t, pool, "project", "reembed-code-index-scope", nil, principal.ID)

	var memID uuid.UUID
	if err := pool.QueryRow(ctx, `
		INSERT INTO memories (memory_type, scope_id, author_id, content, content_kind, is_active)
		VALUES ('semantic', $1, $2, 'fn main() {}', 'code', true)
		RETURNING id
	`, scope.ID, principal.ID).Scan(&memID); err != nil {
		t.Fatalf("insert code memory: %v", err)
	}
	if _, err := pool.Exec(ctx, `
		INSERT INTO embedding_index (object_type, object_id, model_id, status, retry_count)
		VALUES ('memory', $1, $2, 'pending', 0)
		ON CONFLICT (object_type, object_id, model_id)
		DO UPDATE SET status='pending', retry_count=0, last_error=NULL
	`, memID, codeModelID); err != nil {
		t.Fatalf("seed embedding_index pending row: %v", err)
	}

	j := NewReembedJob(pool, svc, 64)
	if err := j.RunCode(ctx); err != nil {
		t.Fatalf("RunCode: %v", err)
	}

	var (
		afterModelID *uuid.UUID
		status       string
		retryCount   int
		lastError    *string
	)
	if err := pool.QueryRow(ctx, `
		SELECT embedding_code_model_id FROM memories WHERE id = $1
	`, memID).Scan(&afterModelID); err != nil {
		t.Fatalf("scan memory after RunCode: %v", err)
	}
	if afterModelID == nil || *afterModelID != codeModelID {
		t.Fatalf("embedding_code_model_id = %v, want %v", afterModelID, codeModelID)
	}
	if err := pool.QueryRow(ctx, `
		SELECT status, retry_count, last_error
		FROM embedding_index
		WHERE object_type='memory' AND object_id=$1 AND model_id=$2
	`, memID, codeModelID).Scan(&status, &retryCount, &lastError); err != nil {
		t.Fatalf("scan embedding_index after RunCode: %v", err)
	}
	if status != "ready" {
		t.Fatalf("status = %q, want ready", status)
	}
	if retryCount != 0 {
		t.Fatalf("retry_count = %d, want 0", retryCount)
	}
	if lastError != nil {
		t.Fatalf("last_error = %v, want NULL", *lastError)
	}
}

func TestReembedJob_RunCode_FailureIncrementsRetryAndEventuallyFailed(t *testing.T) {
	t.Parallel()
	pool := testhelper.NewTestPool(t)
	svc := providers.NewServiceFromEmbedders(providers.NewFakeEmbedder(4), &failingEmbedder{})
	ctx := context.Background()

	codeModel, err := db.RegisterEmbeddingModel(ctx, pool, db.RegisterEmbeddingModelParams{
		Slug:          "reembed-code-fail-" + uuid.NewString(),
		Provider:      "ollama",
		ServiceURL:    "http://localhost:11434",
		ProviderModel: "nomic-embed-text",
		Dimensions:    4,
		ContentType:   "code",
		Activate:      true,
	})
	if err != nil {
		t.Fatalf("register code model: %v", err)
	}
	codeModelID := codeModel.ID

	principal := testhelper.CreateTestPrincipal(t, pool, "user", "reembed-code-fail-user")
	scope := testhelper.CreateTestScope(t, pool, "project", "reembed-code-fail-scope", nil, principal.ID)

	var memID uuid.UUID
	if err := pool.QueryRow(ctx, `
		INSERT INTO memories (memory_type, scope_id, author_id, content, content_kind, is_active)
		VALUES ('semantic', $1, $2, 'fn main() {}', 'code', true)
		RETURNING id
	`, scope.ID, principal.ID).Scan(&memID); err != nil {
		t.Fatalf("insert code memory: %v", err)
	}
	if _, err := pool.Exec(ctx, `
		INSERT INTO embedding_index (object_type, object_id, model_id, status, retry_count)
		VALUES ('memory', $1, $2, 'pending', 2)
		ON CONFLICT (object_type, object_id, model_id)
		DO UPDATE SET status='pending', retry_count=2, last_error=NULL
	`, memID, codeModelID); err != nil {
		t.Fatalf("seed embedding_index pending row: %v", err)
	}

	j := NewReembedJob(pool, svc, 64)
	if err := j.RunCode(ctx); err != nil {
		t.Fatalf("RunCode: %v", err)
	}

	var (
		status     string
		retryCount int
		lastError  *string
	)
	if err := pool.QueryRow(ctx, `
		SELECT status, retry_count, last_error
		FROM embedding_index
		WHERE object_type='memory' AND object_id=$1 AND model_id=$2
	`, memID, codeModelID).Scan(&status, &retryCount, &lastError); err != nil {
		t.Fatalf("scan embedding_index after RunCode: %v", err)
	}
	if status != "failed" {
		t.Fatalf("status = %q, want failed", status)
	}
	if retryCount != 3 {
		t.Fatalf("retry_count = %d, want 3", retryCount)
	}
	if lastError == nil || *lastError == "" {
		t.Fatal("last_error should be populated on failed embedding row")
	}
}
