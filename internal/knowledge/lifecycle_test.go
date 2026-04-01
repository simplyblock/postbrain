package knowledge

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"
	"github.com/simplyblock/postbrain/internal/db"
)

// fakeLifecycleMembership implements membershipChecker for unit tests.
type fakeLifecycleMembership struct {
	isAdmin bool
	err     error
}

func (f *fakeLifecycleMembership) IsScopeAdmin(_ context.Context, _, _ uuid.UUID) (bool, error) {
	return f.isAdmin, f.err
}

// noopLifecycleDB satisfies the full lifecycleDB interface with no-ops.
// Embed this in test fakes so that adding a new method to lifecycleDB only
// requires updating noopLifecycleDB, not every individual fake.
type noopLifecycleDB struct{}

func (noopLifecycleDB) getArtifact(_ context.Context, _ uuid.UUID) (*db.KnowledgeArtifact, error) {
	return nil, nil
}
func (noopLifecycleDB) updateArtifactStatus(_ context.Context, _ uuid.UUID, _ string, _, _ interface{}) error {
	return nil
}
func (noopLifecycleDB) createEndorsement(_ context.Context, _, _ uuid.UUID, _ *string) (*db.KnowledgeEndorsement, error) {
	return &db.KnowledgeEndorsement{ID: uuid.New()}, nil
}
func (noopLifecycleDB) incrementEndorsementCount(_ context.Context, _ uuid.UUID) error { return nil }
func (noopLifecycleDB) getArtifactAfterEndorse(_ context.Context, _ uuid.UUID) (*db.KnowledgeArtifact, error) {
	return nil, nil
}
func (noopLifecycleDB) snapshotArtifactVersion(_ context.Context, _ *db.KnowledgeHistory) error {
	return nil
}
func (noopLifecycleDB) flagDigestsStaleness(_ context.Context, _ uuid.UUID, _ string, _ float64, _ []byte) error {
	return nil
}
func (noopLifecycleDB) deleteArtifactEntityLinks(_ context.Context, _ uuid.UUID) error { return nil }
func (noopLifecycleDB) nullPreviousVersionRefs(_ context.Context, _ uuid.UUID) error   { return nil }
func (noopLifecycleDB) nullPromotionRequestArtifactRef(_ context.Context, _ uuid.UUID) error {
	return nil
}
func (noopLifecycleDB) resetPromotedMemoryStatus(_ context.Context, _ uuid.UUID) error { return nil }
func (noopLifecycleDB) deleteArtifact(_ context.Context, _ uuid.UUID) error            { return nil }

// fakeLifecycleDB embeds noopLifecycleDB and overrides only the methods that
// tests need to observe or control.
type fakeLifecycleDB struct {
	noopLifecycleDB
	artifact                      *db.KnowledgeArtifact
	statusUpdated                 string
	snapshotted                   bool
	endorsed                      bool
	uniqueViolate                 bool // if true, createEndorsement returns the idempotent sentinel
	entityLinksDeleted            bool
	nulledPreviousVersionRefs     bool
	nulledPromotionRequestRef     bool
	resetPromotedMemoryStatusDone bool
	artifactDeleted               bool
}

func (f *fakeLifecycleDB) getArtifact(_ context.Context, _ uuid.UUID) (*db.KnowledgeArtifact, error) {
	return f.artifact, nil
}

func (f *fakeLifecycleDB) updateArtifactStatus(_ context.Context, _ uuid.UUID, status string, _, _ interface{}) error {
	f.statusUpdated = status
	if f.artifact != nil {
		f.artifact.Status = status
	}
	return nil
}

func (f *fakeLifecycleDB) createEndorsement(_ context.Context, _, _ uuid.UUID, _ *string) (*db.KnowledgeEndorsement, error) {
	if f.uniqueViolate {
		return nil, errDuplicateEndorsement
	}
	f.endorsed = true
	return &db.KnowledgeEndorsement{ID: uuid.New()}, nil
}

func (f *fakeLifecycleDB) incrementEndorsementCount(_ context.Context, _ uuid.UUID) error {
	if f.artifact != nil {
		f.artifact.EndorsementCount++
	}
	return nil
}

func (f *fakeLifecycleDB) getArtifactAfterEndorse(_ context.Context, _ uuid.UUID) (*db.KnowledgeArtifact, error) {
	return f.artifact, nil
}

func (f *fakeLifecycleDB) snapshotArtifactVersion(_ context.Context, _ *db.KnowledgeHistory) error {
	f.snapshotted = true
	return nil
}

func (f *fakeLifecycleDB) deleteArtifactEntityLinks(_ context.Context, _ uuid.UUID) error {
	f.entityLinksDeleted = true
	return nil
}

func (f *fakeLifecycleDB) nullPreviousVersionRefs(_ context.Context, _ uuid.UUID) error {
	f.nulledPreviousVersionRefs = true
	return nil
}

func (f *fakeLifecycleDB) nullPromotionRequestArtifactRef(_ context.Context, _ uuid.UUID) error {
	f.nulledPromotionRequestRef = true
	return nil
}

func (f *fakeLifecycleDB) resetPromotedMemoryStatus(_ context.Context, _ uuid.UUID) error {
	f.resetPromotedMemoryStatusDone = true
	return nil
}

func (f *fakeLifecycleDB) deleteArtifact(_ context.Context, _ uuid.UUID) error {
	f.artifactDeleted = true
	f.artifact = nil
	return nil
}

func newTestLifecycle(artifact *db.KnowledgeArtifact, isAdmin bool) (*Lifecycle, *fakeLifecycleDB) {
	fdb := &fakeLifecycleDB{artifact: artifact}
	lc := &Lifecycle{
		membership: &fakeLifecycleMembership{isAdmin: isAdmin},
		dbOps:      fdb,
	}
	return lc, fdb
}

// TestSubmitForReview_ForbiddenForNonAuthorNonAdmin verifies that a caller who is
// neither the author nor a scope admin cannot submit an artifact for review.
func TestSubmitForReview_ForbiddenForNonAuthorNonAdmin(t *testing.T) {
	t.Parallel()
	authorID := uuid.New()
	artifact := &db.KnowledgeArtifact{
		ID:           uuid.New(),
		AuthorID:     authorID,
		OwnerScopeID: uuid.New(),
		Status:       "draft",
	}
	lc, _ := newTestLifecycle(artifact, false /* not admin */)

	callerID := uuid.New() // different from authorID
	err := lc.SubmitForReview(context.Background(), artifact.ID, callerID)
	if !errors.Is(err, ErrForbidden) {
		t.Errorf("expected ErrForbidden, got %v", err)
	}
}

// TestSubmitForReview_InvalidTransitionFromPublished verifies that attempting to
// submit a published artifact for review returns ErrInvalidTransition.
func TestSubmitForReview_InvalidTransitionFromPublished(t *testing.T) {
	t.Parallel()
	authorID := uuid.New()
	artifact := &db.KnowledgeArtifact{
		ID:           uuid.New(),
		AuthorID:     authorID,
		OwnerScopeID: uuid.New(),
		Status:       "published",
	}
	lc, _ := newTestLifecycle(artifact, false)

	err := lc.SubmitForReview(context.Background(), artifact.ID, authorID)
	if !errors.Is(err, ErrInvalidTransition) {
		t.Errorf("expected ErrInvalidTransition, got %v", err)
	}
}

// TestEndorse_SelfEndorsement verifies that the author cannot endorse their own artifact.
func TestEndorse_SelfEndorsement(t *testing.T) {
	t.Parallel()
	authorID := uuid.New()
	artifact := &db.KnowledgeArtifact{
		ID:             uuid.New(),
		AuthorID:       authorID,
		Status:         "in_review",
		ReviewRequired: 1,
	}
	lc, _ := newTestLifecycle(artifact, false)

	_, err := lc.Endorse(context.Background(), artifact.ID, authorID, nil)
	if !errors.Is(err, ErrSelfEndorsement) {
		t.Errorf("expected ErrSelfEndorsement, got %v", err)
	}
}

// TestEndorse_NotInReview verifies that endorsing a non-in_review artifact fails.
func TestEndorse_NotInReview(t *testing.T) {
	t.Parallel()
	artifact := &db.KnowledgeArtifact{
		ID:             uuid.New(),
		AuthorID:       uuid.New(),
		Status:         "draft",
		ReviewRequired: 1,
	}
	lc, _ := newTestLifecycle(artifact, false)

	_, err := lc.Endorse(context.Background(), artifact.ID, uuid.New(), nil)
	if !errors.Is(err, ErrNotReviewable) {
		t.Errorf("expected ErrNotReviewable, got %v", err)
	}
}

// TestEndorse_AutoPublish verifies that reaching the endorsement threshold auto-publishes.
func TestEndorse_AutoPublish(t *testing.T) {
	t.Parallel()
	artifact := &db.KnowledgeArtifact{
		ID:               uuid.New(),
		AuthorID:         uuid.New(),
		Status:           "in_review",
		ReviewRequired:   1,
		EndorsementCount: 0,
	}
	lc, fdb := newTestLifecycle(artifact, false)

	result, err := lc.Endorse(context.Background(), artifact.ID, uuid.New(), nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.AutoPublished {
		t.Error("expected auto-publish to fire")
	}
	if fdb.statusUpdated != "published" {
		t.Errorf("expected statusUpdated=published, got %s", fdb.statusUpdated)
	}
	if !fdb.snapshotted {
		t.Error("expected snapshot to be taken on auto-publish")
	}
}

// TestEndorse_AdminCanEndorseOwnArtifact verifies that a scope admin is not blocked
// by the self-endorsement guard.
func TestEndorse_AdminCanEndorseOwnArtifact(t *testing.T) {
	t.Parallel()
	authorID := uuid.New()
	artifact := &db.KnowledgeArtifact{
		ID:             uuid.New(),
		AuthorID:       authorID,
		OwnerScopeID:   uuid.New(),
		Status:         "in_review",
		ReviewRequired: 2, // needs 2 endorsements so it won't auto-publish here
	}
	lc, _ := newTestLifecycle(artifact, true /* is admin */)

	_, err := lc.Endorse(context.Background(), artifact.ID, authorID, nil)
	if err != nil {
		t.Errorf("admin self-endorsement should be allowed, got %v", err)
	}
}

// TestEndorse_AdminCanEndorseAnyStatus verifies that a scope admin can endorse
// an artifact regardless of its current status.
func TestEndorse_AdminCanEndorseAnyStatus(t *testing.T) {
	t.Parallel()
	for _, status := range []string{"draft", "published", "deprecated"} {
		status := status
		t.Run(status, func(t *testing.T) {
			t.Parallel()
			artifact := &db.KnowledgeArtifact{
				ID:             uuid.New(),
				AuthorID:       uuid.New(),
				OwnerScopeID:   uuid.New(),
				Status:         status,
				ReviewRequired: 2,
			}
			lc, _ := newTestLifecycle(artifact, true /* is admin */)

			_, err := lc.Endorse(context.Background(), artifact.ID, uuid.New(), nil)
			if err != nil {
				t.Errorf("admin should be able to endorse %s artifact, got %v", status, err)
			}
		})
	}
}

// TestEndorse_NonAdminSelfEndorsementStillBlocked verifies the guard still applies
// for non-admins.
func TestEndorse_NonAdminSelfEndorsementStillBlocked(t *testing.T) {
	t.Parallel()
	authorID := uuid.New()
	artifact := &db.KnowledgeArtifact{
		ID:             uuid.New(),
		AuthorID:       authorID,
		OwnerScopeID:   uuid.New(),
		Status:         "in_review",
		ReviewRequired: 1,
	}
	lc, _ := newTestLifecycle(artifact, false /* not admin */)

	_, err := lc.Endorse(context.Background(), artifact.ID, authorID, nil)
	if !errors.Is(err, ErrSelfEndorsement) {
		t.Errorf("expected ErrSelfEndorsement for non-admin, got %v", err)
	}
}

// TestDeprecate_ForbiddenForNonAdmin verifies that a non-admin cannot deprecate an artifact.
func TestDeprecate_ForbiddenForNonAdmin(t *testing.T) {
	t.Parallel()
	artifact := &db.KnowledgeArtifact{
		ID:           uuid.New(),
		OwnerScopeID: uuid.New(),
		Status:       "published",
	}
	lc, _ := newTestLifecycle(artifact, false /* not admin */)

	err := lc.Deprecate(context.Background(), artifact.ID, uuid.New())
	if !errors.Is(err, ErrForbidden) {
		t.Errorf("expected ErrForbidden, got %v", err)
	}
}

// TestDeprecate_RemovesEntityLinks verifies that deprecating an artifact removes its graph links.
func TestDeprecate_RemovesEntityLinks(t *testing.T) {
	t.Parallel()
	artifact := &db.KnowledgeArtifact{
		ID:           uuid.New(),
		OwnerScopeID: uuid.New(),
		Status:       "published",
	}
	lc, fdb := newTestLifecycle(artifact, true /* admin */)

	if err := lc.Deprecate(context.Background(), artifact.ID, uuid.New()); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !fdb.entityLinksDeleted {
		t.Error("expected artifact entity links to be deleted on deprecation")
	}
}

// ── RetractToDraft ────────────────────────────────────────────────────────────

func TestRetractToDraft_ArtifactNotFound(t *testing.T) {
	t.Parallel()
	lc, _ := newTestLifecycle(nil /* artifact not found */, false)
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
			artifact := &db.KnowledgeArtifact{
				ID:           uuid.New(),
				AuthorID:     uuid.New(),
				OwnerScopeID: uuid.New(),
				Status:       status,
			}
			lc, _ := newTestLifecycle(artifact, false)
			err := lc.RetractToDraft(context.Background(), artifact.ID, artifact.AuthorID)
			if !errors.Is(err, ErrInvalidTransition) {
				t.Errorf("status=%s: expected ErrInvalidTransition, got %v", status, err)
			}
		})
	}
}

func TestRetractToDraft_AuthorCanRetract(t *testing.T) {
	t.Parallel()
	authorID := uuid.New()
	artifact := &db.KnowledgeArtifact{
		ID:           uuid.New(),
		AuthorID:     authorID,
		OwnerScopeID: uuid.New(),
		Status:       "in_review",
	}
	lc, fdb := newTestLifecycle(artifact, false /* not admin */)

	err := lc.RetractToDraft(context.Background(), artifact.ID, authorID)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if fdb.statusUpdated != "draft" {
		t.Errorf("expected statusUpdated=draft, got %q", fdb.statusUpdated)
	}
}

func TestRetractToDraft_NonAuthorNonAdminForbidden(t *testing.T) {
	t.Parallel()
	artifact := &db.KnowledgeArtifact{
		ID:           uuid.New(),
		AuthorID:     uuid.New(),
		OwnerScopeID: uuid.New(),
		Status:       "in_review",
	}
	lc, _ := newTestLifecycle(artifact, false /* not admin */)

	err := lc.RetractToDraft(context.Background(), artifact.ID, uuid.New() /* different caller */)
	if !errors.Is(err, ErrForbidden) {
		t.Errorf("expected ErrForbidden, got %v", err)
	}
}

func TestRetractToDraft_AdminCanRetract(t *testing.T) {
	t.Parallel()
	artifact := &db.KnowledgeArtifact{
		ID:           uuid.New(),
		AuthorID:     uuid.New(),
		OwnerScopeID: uuid.New(),
		Status:       "in_review",
	}
	lc, fdb := newTestLifecycle(artifact, true /* admin */)

	err := lc.RetractToDraft(context.Background(), artifact.ID, uuid.New() /* non-author admin */)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if fdb.statusUpdated != "draft" {
		t.Errorf("expected statusUpdated=draft, got %q", fdb.statusUpdated)
	}
}

// ── Republish ─────────────────────────────────────────────────────────────────

func TestRepublish_ArtifactNotFound(t *testing.T) {
	t.Parallel()
	lc, _ := newTestLifecycle(nil, true)
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
			artifact := &db.KnowledgeArtifact{
				ID:           uuid.New(),
				OwnerScopeID: uuid.New(),
				Status:       status,
			}
			lc, _ := newTestLifecycle(artifact, true)
			err := lc.Republish(context.Background(), artifact.ID, uuid.New())
			if !errors.Is(err, ErrInvalidTransition) {
				t.Errorf("status=%s: expected ErrInvalidTransition, got %v", status, err)
			}
		})
	}
}

func TestRepublish_NonAdminForbidden(t *testing.T) {
	t.Parallel()
	artifact := &db.KnowledgeArtifact{
		ID:           uuid.New(),
		OwnerScopeID: uuid.New(),
		Status:       "deprecated",
	}
	lc, _ := newTestLifecycle(artifact, false /* not admin */)

	err := lc.Republish(context.Background(), artifact.ID, uuid.New())
	if !errors.Is(err, ErrForbidden) {
		t.Errorf("expected ErrForbidden, got %v", err)
	}
}

func TestRepublish_AdminTransitionsToPublished(t *testing.T) {
	t.Parallel()
	artifact := &db.KnowledgeArtifact{
		ID:           uuid.New(),
		OwnerScopeID: uuid.New(),
		Status:       "deprecated",
	}
	lc, fdb := newTestLifecycle(artifact, true /* admin */)

	err := lc.Republish(context.Background(), artifact.ID, uuid.New())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if fdb.statusUpdated != "published" {
		t.Errorf("expected statusUpdated=published, got %q", fdb.statusUpdated)
	}
}

// ── Delete ────────────────────────────────────────────────────────────────────

func TestDelete_NonAdminForbidden(t *testing.T) {
	t.Parallel()
	artifact := &db.KnowledgeArtifact{
		ID:           uuid.New(),
		OwnerScopeID: uuid.New(),
		Status:       "published",
	}
	lc, _ := newTestLifecycle(artifact, false /* not admin */)

	err := lc.Delete(context.Background(), artifact.ID, uuid.New())
	if !errors.Is(err, ErrForbidden) {
		t.Errorf("expected ErrForbidden, got %v", err)
	}
}

func TestDelete_ArtifactNotFound(t *testing.T) {
	t.Parallel()
	lc, _ := newTestLifecycle(nil, true)
	err := lc.Delete(context.Background(), uuid.New(), uuid.New())
	if !errors.Is(err, ErrInvalidTransition) {
		t.Errorf("expected ErrInvalidTransition, got %v", err)
	}
}

func TestDelete_AdminCascadesAllPreDeleteSteps(t *testing.T) {
	t.Parallel()
	artifact := &db.KnowledgeArtifact{
		ID:           uuid.New(),
		OwnerScopeID: uuid.New(),
		Status:       "published",
	}
	lc, fdb := newTestLifecycle(artifact, true /* admin */)

	err := lc.Delete(context.Background(), artifact.ID, uuid.New())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !fdb.nulledPreviousVersionRefs {
		t.Error("expected nullPreviousVersionRefs to be called")
	}
	if !fdb.nulledPromotionRequestRef {
		t.Error("expected nullPromotionRequestArtifactRef to be called")
	}
	if !fdb.resetPromotedMemoryStatusDone {
		t.Error("expected resetPromotedMemoryStatus to be called")
	}
	if !fdb.artifactDeleted {
		t.Error("expected deleteArtifact to be called")
	}
}

// ── EmergencyRollback ─────────────────────────────────────────────────────────

// TestEmergencyRollback_ClearsPublishedAt verifies that EmergencyRollback sets status=draft.
func TestEmergencyRollback_ClearsPublishedAt(t *testing.T) {
	t.Parallel()
	artifact := &db.KnowledgeArtifact{
		ID:           uuid.New(),
		OwnerScopeID: uuid.New(),
		Status:       "published",
	}
	lc, fdb := newTestLifecycle(artifact, true /* is admin */)

	err := lc.EmergencyRollback(context.Background(), artifact.ID, uuid.New())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if fdb.statusUpdated != "draft" {
		t.Errorf("expected statusUpdated=draft, got %s", fdb.statusUpdated)
	}
}
