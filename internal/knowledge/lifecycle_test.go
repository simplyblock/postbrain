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

// fakeLifecycleDB implements lifecycleDB for unit tests.
type fakeLifecycleDB struct {
	artifact      *db.KnowledgeArtifact
	statusUpdated string
	snapshotted   bool
	endorsed      bool
	uniqueViolate bool // if true, CreateEndorsement returns a 23505 error
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
		// Simulate a unique-constraint violation by returning the idempotent sentinel.
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

func (f *fakeLifecycleDB) snapshotArtifactVersion(_ context.Context, h *db.KnowledgeHistory) error {
	f.snapshotted = true
	return nil
}

func (f *fakeLifecycleDB) flagDigestsStaleness(_ context.Context, _ uuid.UUID, _ string, _ float64, _ []byte) error {
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
