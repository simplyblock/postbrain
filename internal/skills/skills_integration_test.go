//go:build integration

package skills_test

import (
	"encoding/json"
	"errors"
	"testing"

	"github.com/simplyblock/postbrain/internal/db"
	"github.com/simplyblock/postbrain/internal/skills"
	"github.com/simplyblock/postbrain/internal/testhelper"
)

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
	path, err := skills.Install(skill, "claude-code", workdir)
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
