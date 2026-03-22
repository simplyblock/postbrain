package skills

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/google/uuid"
	"github.com/simplyblock/postbrain/internal/db"
	"github.com/simplyblock/postbrain/internal/embedding"
)

// fakeEmbedder satisfies the embedding.Embedder interface without a real model.
type fakeEmbedder struct{}

func (f *fakeEmbedder) Embed(_ context.Context, _ string) ([]float32, error) {
	return []float32{0.1, 0.2, 0.3}, nil
}
func (f *fakeEmbedder) EmbedBatch(_ context.Context, texts []string) ([][]float32, error) {
	out := make([][]float32, len(texts))
	for i := range texts {
		out[i] = []float32{0.1, 0.2, 0.3}
	}
	return out, nil
}
func (f *fakeEmbedder) ModelSlug() string { return "fake" }
func (f *fakeEmbedder) Dimensions() int   { return 3 }

// fakeDB stores the last CreateSkill call.
type fakeDB struct {
	created *db.Skill
}

func (f *fakeDB) createSkill(_ context.Context, s *db.Skill) (*db.Skill, error) {
	s.ID = uuid.New()
	f.created = s
	return s, nil
}

// newTestStore creates a Store with a fake creator and fake embedder for unit tests.
func newTestStore(fdb skillCreator) *Store {
	svc := embedding.NewServiceFromEmbedders(&fakeEmbedder{}, nil)
	return &Store{
		creator: fdb,
		svc:     svc,
	}
}

func TestCreate_DefaultAgentTypes(t *testing.T) {
	t.Parallel()
	fdb := &fakeDB{}
	s := newTestStore(fdb)
	input := CreateInput{
		ScopeID:    uuid.New(),
		AuthorID:   uuid.New(),
		Slug:       "test-skill",
		Name:       "Test Skill",
		Body:       "Do the thing.",
		Visibility: "team",
	}
	skill, err := s.Create(context.Background(), input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(skill.AgentTypes) != 1 || skill.AgentTypes[0] != "any" {
		t.Errorf("expected AgentTypes=[any], got %v", skill.AgentTypes)
	}
}

func TestCreate_DefaultReviewRequired(t *testing.T) {
	t.Parallel()
	fdb := &fakeDB{}
	s := newTestStore(fdb)
	input := CreateInput{
		ScopeID:    uuid.New(),
		AuthorID:   uuid.New(),
		Slug:       "test-skill",
		Name:       "Test Skill",
		Body:       "Do the thing.",
		Visibility: "team",
		// ReviewRequired intentionally left at zero
	}
	skill, err := s.Create(context.Background(), input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if skill.ReviewRequired != 1 {
		t.Errorf("expected ReviewRequired=1, got %d", skill.ReviewRequired)
	}
}

func TestCreate_ParametersSerialized(t *testing.T) {
	t.Parallel()
	fdb := &fakeDB{}
	s := newTestStore(fdb)
	input := CreateInput{
		ScopeID:    uuid.New(),
		AuthorID:   uuid.New(),
		Slug:       "test-skill",
		Name:       "Test Skill",
		Body:       "Do the thing with $PARAM.",
		Visibility: "team",
		Parameters: []db.SkillParameter{
			{Name: "param", Type: "string", Required: true, Description: "A param"},
		},
	}
	skill, err := s.Create(context.Background(), input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	var params []db.SkillParameter
	if err := json.Unmarshal(skill.Parameters, &params); err != nil {
		t.Fatalf("Parameters is not valid JSON: %v (raw: %s)", err, skill.Parameters)
	}
	if len(params) != 1 || params[0].Name != "param" {
		t.Errorf("unexpected params: %+v", params)
	}
}
