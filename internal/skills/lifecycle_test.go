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
	isAdmin       bool
	isSystemAdmin bool
	err           error
}

func (f *fakeMembership) IsScopeAdmin(_ context.Context, _, _ uuid.UUID) (bool, error) {
	return f.isAdmin, f.err
}

func (f *fakeMembership) IsSystemAdmin(_ context.Context, _ uuid.UUID) (bool, error) {
	return f.isSystemAdmin, f.err
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
	if f.skill != nil {
		f.skill.Status = status
	}
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

// newTestLifecycleSystemAdmin creates a Lifecycle where the caller is a system admin
// (but not a scope admin via membership) to verify system-admin bypass paths.
func newTestLifecycleSystemAdmin(skill *db.Skill, endorsements int) (*Lifecycle, *fakeLifecycleDB) {
	fdb := newFakeLifecycleDB(skill, endorsements)
	lc := &Lifecycle{
		membership: &fakeMembership{isSystemAdmin: true, isAdmin: false},
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

// ── SubmitForReview ───────────────────────────────────────────────────────────

func TestSubmitForReview_NilSkill(t *testing.T) {
	t.Parallel()
	lc, _ := newTestLifecycle(nil, 0, false)
	err := lc.SubmitForReview(context.Background(), uuid.New(), uuid.New())
	if !errors.Is(err, ErrInvalidTransition) {
		t.Errorf("expected ErrInvalidTransition, got %v", err)
	}
}

func TestSubmitForReview_WrongStatus(t *testing.T) {
	t.Parallel()
	for _, status := range []string{"in_review", "published", "deprecated"} {
		status := status
		t.Run(status, func(t *testing.T) {
			t.Parallel()
			skill := &db.Skill{ID: uuid.New(), Status: status}
			lc, _ := newTestLifecycle(skill, 0, false)
			err := lc.SubmitForReview(context.Background(), skill.ID, uuid.New())
			if !errors.Is(err, ErrInvalidTransition) {
				t.Errorf("status=%s: expected ErrInvalidTransition, got %v", status, err)
			}
		})
	}
}

func TestSubmitForReview_DraftTransitionsToInReview(t *testing.T) {
	t.Parallel()
	skill := &db.Skill{ID: uuid.New(), AuthorID: uuid.New(), Status: "draft"}
	lc, fdb := newTestLifecycle(skill, 0, false)
	err := lc.SubmitForReview(context.Background(), skill.ID, skill.AuthorID)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if fdb.statusUpdated != "in_review" {
		t.Errorf("expected statusUpdated=in_review, got %q", fdb.statusUpdated)
	}
}

// ── RetractToDraft ────────────────────────────────────────────────────────────

func TestRetractToDraft_NilSkill(t *testing.T) {
	t.Parallel()
	lc, _ := newTestLifecycle(nil, 0, false)
	err := lc.RetractToDraft(context.Background(), uuid.New(), uuid.New())
	if !errors.Is(err, ErrInvalidTransition) {
		t.Errorf("expected ErrInvalidTransition, got %v", err)
	}
}

func TestRetractToDraft_WrongStatus(t *testing.T) {
	t.Parallel()
	for _, status := range []string{"draft", "published", "deprecated"} {
		status := status
		t.Run(status, func(t *testing.T) {
			t.Parallel()
			skill := &db.Skill{ID: uuid.New(), Status: status}
			lc, _ := newTestLifecycle(skill, 0, false)
			err := lc.RetractToDraft(context.Background(), skill.ID, uuid.New())
			if !errors.Is(err, ErrInvalidTransition) {
				t.Errorf("status=%s: expected ErrInvalidTransition, got %v", status, err)
			}
		})
	}
}

func TestRetractToDraft_InReviewTransitionsToDraft(t *testing.T) {
	t.Parallel()
	skill := &db.Skill{ID: uuid.New(), Status: "in_review"}
	lc, fdb := newTestLifecycle(skill, 0, false)
	err := lc.RetractToDraft(context.Background(), skill.ID, uuid.New())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if fdb.statusUpdated != "draft" {
		t.Errorf("expected statusUpdated=draft, got %q", fdb.statusUpdated)
	}
}

// ── Deprecate ─────────────────────────────────────────────────────────────────

func TestDeprecate_NilSkill(t *testing.T) {
	t.Parallel()
	lc, _ := newTestLifecycle(nil, 0, true)
	err := lc.Deprecate(context.Background(), uuid.New(), uuid.New())
	if !errors.Is(err, ErrInvalidTransition) {
		t.Errorf("expected ErrInvalidTransition, got %v", err)
	}
}

func TestDeprecate_WrongStatus(t *testing.T) {
	t.Parallel()
	for _, status := range []string{"draft", "in_review", "deprecated"} {
		status := status
		t.Run(status, func(t *testing.T) {
			t.Parallel()
			skill := &db.Skill{ID: uuid.New(), ScopeID: uuid.New(), Status: status}
			lc, _ := newTestLifecycle(skill, 0, true)
			err := lc.Deprecate(context.Background(), skill.ID, uuid.New())
			if !errors.Is(err, ErrInvalidTransition) {
				t.Errorf("status=%s: expected ErrInvalidTransition, got %v", status, err)
			}
		})
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

func TestDeprecate_AdminSucceeds(t *testing.T) {
	t.Parallel()
	skill := &db.Skill{
		ID:      uuid.New(),
		ScopeID: uuid.New(),
		Status:  "published",
	}
	lc, fdb := newTestLifecycle(skill, 0, true)
	err := lc.Deprecate(context.Background(), skill.ID, uuid.New())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if fdb.statusUpdated != "deprecated" {
		t.Errorf("expected statusUpdated=deprecated, got %q", fdb.statusUpdated)
	}
}

// ── Republish ─────────────────────────────────────────────────────────────────

func TestRepublish_NilSkill(t *testing.T) {
	t.Parallel()
	lc, _ := newTestLifecycle(nil, 0, true)
	err := lc.Republish(context.Background(), uuid.New(), uuid.New())
	if !errors.Is(err, ErrInvalidTransition) {
		t.Errorf("expected ErrInvalidTransition, got %v", err)
	}
}

func TestRepublish_WrongStatus(t *testing.T) {
	t.Parallel()
	for _, status := range []string{"draft", "in_review", "published"} {
		status := status
		t.Run(status, func(t *testing.T) {
			t.Parallel()
			skill := &db.Skill{ID: uuid.New(), ScopeID: uuid.New(), Status: status}
			lc, _ := newTestLifecycle(skill, 0, true)
			err := lc.Republish(context.Background(), skill.ID, uuid.New())
			if !errors.Is(err, ErrInvalidTransition) {
				t.Errorf("status=%s: expected ErrInvalidTransition, got %v", status, err)
			}
		})
	}
}

func TestRepublish_NonAdmin(t *testing.T) {
	t.Parallel()
	skill := &db.Skill{
		ID:      uuid.New(),
		ScopeID: uuid.New(),
		Status:  "deprecated",
	}
	lc, _ := newTestLifecycle(skill, 0, false)
	err := lc.Republish(context.Background(), skill.ID, uuid.New())
	if !errors.Is(err, ErrForbidden) {
		t.Errorf("expected ErrForbidden, got %v", err)
	}
}

func TestRepublish_AdminSucceeds(t *testing.T) {
	t.Parallel()
	skill := &db.Skill{
		ID:      uuid.New(),
		ScopeID: uuid.New(),
		Status:  "deprecated",
	}
	lc, fdb := newTestLifecycle(skill, 0, true)
	err := lc.Republish(context.Background(), skill.ID, uuid.New())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if fdb.statusUpdated != "published" {
		t.Errorf("expected statusUpdated=published, got %q", fdb.statusUpdated)
	}
}

// ── EmergencyRollback ─────────────────────────────────────────────────────────

func TestEmergencyRollback_NilSkill(t *testing.T) {
	t.Parallel()
	lc, _ := newTestLifecycle(nil, 0, true)
	err := lc.EmergencyRollback(context.Background(), uuid.New(), uuid.New())
	if !errors.Is(err, ErrInvalidTransition) {
		t.Errorf("expected ErrInvalidTransition, got %v", err)
	}
}

func TestEmergencyRollback_AlreadyDraft(t *testing.T) {
	t.Parallel()
	skill := &db.Skill{ID: uuid.New(), ScopeID: uuid.New(), Status: "draft"}
	lc, _ := newTestLifecycle(skill, 0, true)
	err := lc.EmergencyRollback(context.Background(), skill.ID, uuid.New())
	if !errors.Is(err, ErrInvalidTransition) {
		t.Errorf("expected ErrInvalidTransition, got %v", err)
	}
}

func TestEmergencyRollback_NonAdmin(t *testing.T) {
	t.Parallel()
	skill := &db.Skill{ID: uuid.New(), ScopeID: uuid.New(), Status: "published"}
	lc, _ := newTestLifecycle(skill, 0, false)
	err := lc.EmergencyRollback(context.Background(), skill.ID, uuid.New())
	if !errors.Is(err, ErrForbidden) {
		t.Errorf("expected ErrForbidden, got %v", err)
	}
}

func TestEmergencyRollback_AdminTransitionsToDraft(t *testing.T) {
	t.Parallel()
	for _, status := range []string{"in_review", "published", "deprecated"} {
		status := status
		t.Run(status, func(t *testing.T) {
			t.Parallel()
			skill := &db.Skill{ID: uuid.New(), ScopeID: uuid.New(), Status: status}
			lc, fdb := newTestLifecycle(skill, 0, true)
			err := lc.EmergencyRollback(context.Background(), skill.ID, uuid.New())
			if err != nil {
				t.Fatalf("status=%s: unexpected error: %v", status, err)
			}
			if fdb.statusUpdated != "draft" {
				t.Errorf("status=%s: expected statusUpdated=draft, got %q", status, fdb.statusUpdated)
			}
		})
	}
}

// ── System admin bypass tests ─────────────────────────────────────────────────

func TestDeprecate_SystemAdminCanDeprecate(t *testing.T) {
	t.Parallel()
	skill := &db.Skill{
		ID:      uuid.New(),
		ScopeID: uuid.New(),
		Status:  "published",
	}
	lc, fdb := newTestLifecycleSystemAdmin(skill, 0)
	err := lc.Deprecate(context.Background(), skill.ID, uuid.New())
	if err != nil {
		t.Fatalf("system admin should be able to deprecate, got %v", err)
	}
	if fdb.statusUpdated != "deprecated" {
		t.Errorf("expected statusUpdated=deprecated, got %q", fdb.statusUpdated)
	}
}

func TestRepublish_SystemAdminCanRepublish(t *testing.T) {
	t.Parallel()
	skill := &db.Skill{
		ID:      uuid.New(),
		ScopeID: uuid.New(),
		Status:  "deprecated",
	}
	lc, fdb := newTestLifecycleSystemAdmin(skill, 0)
	err := lc.Republish(context.Background(), skill.ID, uuid.New())
	if err != nil {
		t.Fatalf("system admin should be able to republish, got %v", err)
	}
	if fdb.statusUpdated != "published" {
		t.Errorf("expected statusUpdated=published, got %q", fdb.statusUpdated)
	}
}

func TestEmergencyRollback_SystemAdminTransitionsToDraft(t *testing.T) {
	t.Parallel()
	for _, status := range []string{"in_review", "published", "deprecated"} {
		status := status
		t.Run(status, func(t *testing.T) {
			t.Parallel()
			skill := &db.Skill{ID: uuid.New(), ScopeID: uuid.New(), Status: status}
			lc, fdb := newTestLifecycleSystemAdmin(skill, 0)
			err := lc.EmergencyRollback(context.Background(), skill.ID, uuid.New())
			if err != nil {
				t.Fatalf("status=%s: system admin should be able to rollback, got %v", status, err)
			}
			if fdb.statusUpdated != "draft" {
				t.Errorf("status=%s: expected statusUpdated=draft, got %q", status, fdb.statusUpdated)
			}
		})
	}
}
