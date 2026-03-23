package knowledge

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/simplyblock/postbrain/internal/db"
)

// Sentinel errors for the knowledge lifecycle state machine.
var (
	ErrSelfEndorsement   = errors.New("knowledge: author cannot endorse their own artifact")
	ErrNotReviewable     = errors.New("knowledge: artifact is not in review state")
	ErrForbidden         = errors.New("knowledge: caller does not have permission for this operation")
	ErrInvalidTransition = errors.New("knowledge: invalid state transition")
)

// errDuplicateEndorsement is an internal sentinel used by the fake in tests;
// the real DB path checks for pgx error code 23505 and converts it.
var errDuplicateEndorsement = errors.New("knowledge: endorsement already exists (idempotent)")

// membershipChecker can determine whether a principal is a scope admin.
type membershipChecker interface {
	IsScopeAdmin(ctx context.Context, principalID, scopeID uuid.UUID) (bool, error)
}

// lifecycleDB abstracts all database calls made by Lifecycle, enabling unit tests.
type lifecycleDB interface {
	getArtifact(ctx context.Context, id uuid.UUID) (*db.KnowledgeArtifact, error)
	updateArtifactStatus(ctx context.Context, id uuid.UUID, status string, publishedAt, deprecatedAt interface{}) error
	createEndorsement(ctx context.Context, artifactID, endorserID uuid.UUID, note *string) (*db.KnowledgeEndorsement, error)
	incrementEndorsementCount(ctx context.Context, artifactID uuid.UUID) error
	getArtifactAfterEndorse(ctx context.Context, id uuid.UUID) (*db.KnowledgeArtifact, error)
	snapshotArtifactVersion(ctx context.Context, h *db.KnowledgeHistory) error
}

// poolLifecycleDB wraps a pgxpool.Pool to implement lifecycleDB.
type poolLifecycleDB struct {
	pool *pgxpool.Pool
}

func (p *poolLifecycleDB) getArtifact(ctx context.Context, id uuid.UUID) (*db.KnowledgeArtifact, error) {
	return db.GetArtifact(ctx, p.pool, id)
}

func (p *poolLifecycleDB) updateArtifactStatus(ctx context.Context, id uuid.UUID, status string, publishedAt, deprecatedAt interface{}) error {
	var pub, dep *time.Time
	if t, ok := publishedAt.(*time.Time); ok {
		pub = t
	}
	if t, ok := deprecatedAt.(*time.Time); ok {
		dep = t
	}
	return db.UpdateArtifactStatus(ctx, p.pool, id, status, pub, dep)
}

func (p *poolLifecycleDB) createEndorsement(ctx context.Context, artifactID, endorserID uuid.UUID, note *string) (*db.KnowledgeEndorsement, error) {
	e, err := db.CreateEndorsement(ctx, p.pool, artifactID, endorserID, note)
	if err != nil {
		// pgx unique violation code 23505 — treat as idempotent.
		if isUniqueViolation(err) {
			return nil, errDuplicateEndorsement
		}
		return nil, err
	}
	return e, nil
}

func (p *poolLifecycleDB) incrementEndorsementCount(ctx context.Context, artifactID uuid.UUID) error {
	return db.IncrementArtifactEndorsementCount(ctx, p.pool, artifactID)
}

func (p *poolLifecycleDB) getArtifactAfterEndorse(ctx context.Context, id uuid.UUID) (*db.KnowledgeArtifact, error) {
	return db.GetArtifact(ctx, p.pool, id)
}

func (p *poolLifecycleDB) snapshotArtifactVersion(ctx context.Context, h *db.KnowledgeHistory) error {
	return db.SnapshotArtifactVersion(ctx, p.pool, h)
}

// isUniqueViolation checks if the error is a PostgreSQL unique-constraint violation (23505).
func isUniqueViolation(err error) bool {
	// pgx wraps PgError; check the SQLState code.
	type pgErr interface {
		SQLState() string
	}
	var pg pgErr
	if errors.As(err, &pg) {
		return pg.SQLState() == "23505"
	}
	return false
}

// Lifecycle manages state transitions for knowledge artifacts.
type Lifecycle struct {
	pool       *pgxpool.Pool
	membership membershipChecker
	dbOps      lifecycleDB
}

// NewLifecycle creates a Lifecycle backed by pool and the given membership checker.
func NewLifecycle(pool *pgxpool.Pool, membership membershipChecker) *Lifecycle {
	return &Lifecycle{
		pool:       pool,
		membership: membership,
		dbOps:      &poolLifecycleDB{pool: pool},
	}
}

// EndorseResult carries the outcome of an Endorse call.
type EndorseResult struct {
	EndorsementCount int
	Status           string
	AutoPublished    bool
}

// SubmitForReview transitions an artifact from draft → in_review.
// The caller must be the author or a scope admin.
func (l *Lifecycle) SubmitForReview(ctx context.Context, artifactID, callerID uuid.UUID) error {
	artifact, err := l.dbOps.getArtifact(ctx, artifactID)
	if err != nil {
		return err
	}
	if artifact == nil {
		return ErrInvalidTransition
	}
	if artifact.Status != "draft" {
		return ErrInvalidTransition
	}
	if artifact.AuthorID != callerID {
		ok, err := l.membership.IsScopeAdmin(ctx, callerID, artifact.OwnerScopeID)
		if err != nil {
			return err
		}
		if !ok {
			return ErrForbidden
		}
	}
	return l.dbOps.updateArtifactStatus(ctx, artifactID, "in_review", (*time.Time)(nil), (*time.Time)(nil))
}

// RetractToDraft transitions an artifact from in_review → draft.
// The caller must be the author or a scope admin.
func (l *Lifecycle) RetractToDraft(ctx context.Context, artifactID, callerID uuid.UUID) error {
	artifact, err := l.dbOps.getArtifact(ctx, artifactID)
	if err != nil {
		return err
	}
	if artifact == nil {
		return ErrInvalidTransition
	}
	if artifact.Status != "in_review" {
		return ErrInvalidTransition
	}
	if artifact.AuthorID != callerID {
		ok, err := l.membership.IsScopeAdmin(ctx, callerID, artifact.OwnerScopeID)
		if err != nil {
			return err
		}
		if !ok {
			return ErrForbidden
		}
	}
	return l.dbOps.updateArtifactStatus(ctx, artifactID, "draft", (*time.Time)(nil), (*time.Time)(nil))
}

// Endorse records an endorsement and auto-publishes when the review threshold is reached.
// Scope admins bypass the self-endorsement guard and the in_review status requirement.
func (l *Lifecycle) Endorse(ctx context.Context, artifactID, endorserID uuid.UUID, note *string) (*EndorseResult, error) {
	artifact, err := l.dbOps.getArtifact(ctx, artifactID)
	if err != nil {
		return nil, err
	}
	if artifact == nil {
		return nil, ErrInvalidTransition
	}

	isAdmin, err := l.membership.IsScopeAdmin(ctx, endorserID, artifact.OwnerScopeID)
	if err != nil {
		return nil, err
	}

	if !isAdmin {
		if artifact.AuthorID == endorserID {
			return nil, ErrSelfEndorsement
		}
		if artifact.Status != "in_review" {
			return nil, ErrNotReviewable
		}
	}

	_, err = l.dbOps.createEndorsement(ctx, artifactID, endorserID, note)
	if err != nil && !errors.Is(err, errDuplicateEndorsement) {
		return nil, err
	}
	// Only increment denormalized count when a new endorsement was actually created.
	if !errors.Is(err, errDuplicateEndorsement) {
		if err2 := l.dbOps.incrementEndorsementCount(ctx, artifactID); err2 != nil {
			return nil, err2
		}
	}

	// Get fresh artifact to read the current endorsement_count.
	fresh, err := l.dbOps.getArtifactAfterEndorse(ctx, artifactID)
	if err != nil {
		return nil, err
	}
	if fresh == nil {
		fresh = artifact
	}

	result := &EndorseResult{
		EndorsementCount: int(fresh.EndorsementCount),
		Status:           fresh.Status,
	}

	if fresh.EndorsementCount >= fresh.ReviewRequired {
		if err := l.autoPublish(ctx, artifactID, fresh); err != nil {
			return nil, err
		}
		result.Status = "published"
		result.AutoPublished = true
	}

	return result, nil
}

// autoPublish snapshots the current version and transitions the artifact to published.
func (l *Lifecycle) autoPublish(ctx context.Context, artifactID uuid.UUID, artifact *db.KnowledgeArtifact) error {
	if err := l.dbOps.snapshotArtifactVersion(ctx, &db.KnowledgeHistory{
		ArtifactID: artifactID,
		Version:    artifact.Version,
		Content:    artifact.Content,
		Summary:    artifact.Summary,
		ChangedBy:  artifact.AuthorID,
	}); err != nil {
		return err
	}
	now := time.Now().UTC()
	return l.dbOps.updateArtifactStatus(ctx, artifactID, "published", &now, (*time.Time)(nil))
}

// Deprecate transitions a published artifact to deprecated; requires scope admin.
func (l *Lifecycle) Deprecate(ctx context.Context, artifactID, callerID uuid.UUID) error {
	artifact, err := l.dbOps.getArtifact(ctx, artifactID)
	if err != nil {
		return err
	}
	if artifact == nil {
		return ErrInvalidTransition
	}
	if artifact.Status != "published" {
		return ErrInvalidTransition
	}
	ok, err := l.membership.IsScopeAdmin(ctx, callerID, artifact.OwnerScopeID)
	if err != nil {
		return err
	}
	if !ok {
		return ErrForbidden
	}
	now := time.Now().UTC()
	return l.dbOps.updateArtifactStatus(ctx, artifactID, "deprecated", (*time.Time)(nil), &now)
}

// Republish transitions a deprecated artifact back to published; requires scope admin.
func (l *Lifecycle) Republish(ctx context.Context, artifactID, callerID uuid.UUID) error {
	artifact, err := l.dbOps.getArtifact(ctx, artifactID)
	if err != nil {
		return err
	}
	if artifact == nil {
		return ErrInvalidTransition
	}
	if artifact.Status != "deprecated" {
		return ErrInvalidTransition
	}
	ok, err := l.membership.IsScopeAdmin(ctx, callerID, artifact.OwnerScopeID)
	if err != nil {
		return err
	}
	if !ok {
		return ErrForbidden
	}
	return l.dbOps.updateArtifactStatus(ctx, artifactID, "published", artifact.PublishedAt, (*time.Time)(nil))
}

// EmergencyRollback transitions any non-draft artifact back to draft; requires scope admin.
func (l *Lifecycle) EmergencyRollback(ctx context.Context, artifactID, callerID uuid.UUID) error {
	artifact, err := l.dbOps.getArtifact(ctx, artifactID)
	if err != nil {
		return err
	}
	if artifact == nil {
		return ErrInvalidTransition
	}
	ok, err := l.membership.IsScopeAdmin(ctx, callerID, artifact.OwnerScopeID)
	if err != nil {
		return err
	}
	if !ok {
		return ErrForbidden
	}
	return l.dbOps.updateArtifactStatus(ctx, artifactID, "draft", (*time.Time)(nil), (*time.Time)(nil))
}
