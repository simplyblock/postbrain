package skills

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"
	"github.com/simplyblock/postbrain/internal/db"
)

// fakeMembership implements membershipChecker for unit tests.
type fakeMembership struct {
	isAdmin bool
	err     error
}

func (f *fakeMembership) IsScopeAdmin(_ context.Context, _, _ uuid.UUID) (bool, error) {
	return f.isAdmin, f.err
}

// fakeLifecycleDB wires up the lifecycle operations without a real DB.
type fakeLifecycleDB struct {
	skill            *db.Skill
	endorsements     int
	statusUpdated    string
	endorsementAdded bool
}

func newFakeLifecycleDB(skill *db.Skill, endorsements int) *fakeLifecycleDB {
	return &fakeLifecycleDB{skill: skill, endorsements: endorsements}
}

func (f *fakeLifecycleDB) getSkill(_ context.Context, _ uuid.UUID) (*db.Skill, error) {
	return f.skill, nil
}
func (f *fakeLifecycleDB) updateSkillStatus(_ context.Context, _ uuid.UUID, status string, _, _ interface{}) error {
	f.statusUpdated = status
	f.skill.Status = status
	return nil
}
func (f *fakeLifecycleDB) getSkillEndorsementByEndorser(_ context.Context, _, _ uuid.UUID) (*db.SkillEndorsement, error) {
	return nil, nil
}
func (f *fakeLifecycleDB) createSkillEndorsement(_ context.Context, _, _ uuid.UUID, _ *string) (*db.SkillEndorsement, error) {
	f.endorsementAdded = true
	f.endorsements++
	return &db.SkillEndorsement{ID: uuid.New()}, nil
}
func (f *fakeLifecycleDB) countSkillEndorsements(_ context.Context, _ uuid.UUID) (int, error) {
	return f.endorsements, nil
}

func newTestLifecycle(skill *db.Skill, endorsements int, isAdmin bool) (*Lifecycle, *fakeLifecycleDB) {
	fdb := newFakeLifecycleDB(skill, endorsements)
	lc := &Lifecycle{
		membership: &fakeMembership{isAdmin: isAdmin},
		dbOps:      fdb,
	}
	return lc, fdb
}

func TestEndorse_SelfEndorsement(t *testing.T) {
	t.Parallel()
	authorID := uuid.New()
	skill := &db.Skill{
		ID:             uuid.New(),
		AuthorID:       authorID,
		Status:         "in_review",
		ReviewRequired: 1,
	}
	lc, _ := newTestLifecycle(skill, 0, false)
	_, err := lc.Endorse(context.Background(), skill.ID, authorID, nil)
	if !errors.Is(err, ErrSelfEndorsement) {
		t.Errorf("expected ErrSelfEndorsement, got %v", err)
	}
}

func TestEndorse_NotInReview(t *testing.T) {
	t.Parallel()
	skill := &db.Skill{
		ID:             uuid.New(),
		AuthorID:       uuid.New(),
		Status:         "draft",
		ReviewRequired: 1,
	}
	lc, _ := newTestLifecycle(skill, 0, false)
	_, err := lc.Endorse(context.Background(), skill.ID, uuid.New(), nil)
	if !errors.Is(err, ErrNotReviewable) {
		t.Errorf("expected ErrNotReviewable, got %v", err)
	}
}

func TestEndorse_AutoPublish(t *testing.T) {
	t.Parallel()
	skill := &db.Skill{
		ID:             uuid.New(),
		AuthorID:       uuid.New(),
		Status:         "in_review",
		ReviewRequired: 1,
	}
	lc, fdb := newTestLifecycle(skill, 0, false)
	result, err := lc.Endorse(context.Background(), skill.ID, uuid.New(), nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.AutoPublished {
		t.Error("expected auto-publish to fire")
	}
	if fdb.statusUpdated != "published" {
		t.Errorf("expected status=published, got %s", fdb.statusUpdated)
	}
}

func TestDeprecate_NonAdmin(t *testing.T) {
	t.Parallel()
	skill := &db.Skill{
		ID:      uuid.New(),
		ScopeID: uuid.New(),
		Status:  "published",
	}
	lc, _ := newTestLifecycle(skill, 0, false)
	err := lc.Deprecate(context.Background(), skill.ID, uuid.New())
	if !errors.Is(err, ErrForbidden) {
		t.Errorf("expected ErrForbidden, got %v", err)
	}
}
