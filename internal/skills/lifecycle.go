package skills

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/simplyblock/postbrain/internal/db"
)

// Sentinel errors for the skill lifecycle state machine.
var (
	ErrSelfEndorsement   = errors.New("skills: author cannot endorse their own skill")
	ErrNotReviewable     = errors.New("skills: skill is not in review")
	ErrForbidden         = errors.New("skills: caller does not have permission")
	ErrInvalidTransition = errors.New("skills: invalid state transition")
)

// membershipChecker can determine whether a principal is a scope admin.
type membershipChecker interface {
	IsScopeAdmin(ctx context.Context, principalID, scopeID uuid.UUID) (bool, error)
}

// lifecycleDB abstracts all database calls made by Lifecycle, enabling unit tests.
type lifecycleDB interface {
	getSkill(ctx context.Context, id uuid.UUID) (*db.Skill, error)
	updateSkillStatus(ctx context.Context, id uuid.UUID, status string, publishedAt, deprecatedAt interface{}) error
	getSkillEndorsementByEndorser(ctx context.Context, skillID, endorserID uuid.UUID) (*db.SkillEndorsement, error)
	createSkillEndorsement(ctx context.Context, skillID, endorserID uuid.UUID, note *string) (*db.SkillEndorsement, error)
	countSkillEndorsements(ctx context.Context, skillID uuid.UUID) (int, error)
}

// poolLifecycleDB wraps a pgxpool.Pool to implement lifecycleDB.
type poolLifecycleDB struct {
	pool *pgxpool.Pool
}

func (p *poolLifecycleDB) getSkill(ctx context.Context, id uuid.UUID) (*db.Skill, error) {
	return db.GetSkill(ctx, p.pool, id)
}
func (p *poolLifecycleDB) updateSkillStatus(ctx context.Context, id uuid.UUID, status string, publishedAt, deprecatedAt interface{}) error {
	var pub, dep *time.Time
	if t, ok := publishedAt.(*time.Time); ok {
		pub = t
	}
	if t, ok := deprecatedAt.(*time.Time); ok {
		dep = t
	}
	return db.UpdateSkillStatus(ctx, p.pool, id, status, pub, dep)
}
func (p *poolLifecycleDB) getSkillEndorsementByEndorser(ctx context.Context, skillID, endorserID uuid.UUID) (*db.SkillEndorsement, error) {
	return db.GetSkillEndorsementByEndorser(ctx, p.pool, skillID, endorserID)
}
func (p *poolLifecycleDB) createSkillEndorsement(ctx context.Context, skillID, endorserID uuid.UUID, note *string) (*db.SkillEndorsement, error) {
	return db.CreateSkillEndorsement(ctx, p.pool, skillID, endorserID, note)
}
func (p *poolLifecycleDB) countSkillEndorsements(ctx context.Context, skillID uuid.UUID) (int, error) {
	return db.CountSkillEndorsements(ctx, p.pool, skillID)
}

// Lifecycle manages state transitions for skills.
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

// SubmitForReview transitions a skill from draft → in_review.
func (l *Lifecycle) SubmitForReview(ctx context.Context, skillID, callerID uuid.UUID) error {
	skill, err := l.dbOps.getSkill(ctx, skillID)
	if err != nil {
		return err
	}
	if skill == nil {
		return ErrInvalidTransition
	}
	if skill.Status != "draft" {
		return ErrInvalidTransition
	}
	return l.dbOps.updateSkillStatus(ctx, skillID, "in_review", (*time.Time)(nil), (*time.Time)(nil))
}

// RetractToDraft transitions a skill from in_review → draft.
func (l *Lifecycle) RetractToDraft(ctx context.Context, skillID, callerID uuid.UUID) error {
	skill, err := l.dbOps.getSkill(ctx, skillID)
	if err != nil {
		return err
	}
	if skill == nil {
		return ErrInvalidTransition
	}
	if skill.Status != "in_review" {
		return ErrInvalidTransition
	}
	return l.dbOps.updateSkillStatus(ctx, skillID, "draft", (*time.Time)(nil), (*time.Time)(nil))
}

// Endorse records an endorsement and auto-publishes when the threshold is reached.
func (l *Lifecycle) Endorse(ctx context.Context, skillID, endorserID uuid.UUID, note *string) (*EndorseResult, error) {
	skill, err := l.dbOps.getSkill(ctx, skillID)
	if err != nil {
		return nil, err
	}
	if skill == nil {
		return nil, ErrInvalidTransition
	}
	if skill.AuthorID == endorserID {
		return nil, ErrSelfEndorsement
	}
	if skill.Status != "in_review" {
		return nil, ErrNotReviewable
	}

	if _, err := l.dbOps.createSkillEndorsement(ctx, skillID, endorserID, note); err != nil {
		return nil, err
	}

	count, err := l.dbOps.countSkillEndorsements(ctx, skillID)
	if err != nil {
		return nil, err
	}

	result := &EndorseResult{
		EndorsementCount: count,
		Status:           skill.Status,
	}

	if count >= skill.ReviewRequired {
		now := time.Now().UTC()
		if err := l.dbOps.updateSkillStatus(ctx, skillID, "published", &now, (*time.Time)(nil)); err != nil {
			return nil, err
		}
		result.Status = "published"
		result.AutoPublished = true
	}

	return result, nil
}

// Deprecate transitions a published skill to deprecated; requires scope admin.
func (l *Lifecycle) Deprecate(ctx context.Context, skillID, callerID uuid.UUID) error {
	skill, err := l.dbOps.getSkill(ctx, skillID)
	if err != nil {
		return err
	}
	if skill == nil {
		return ErrInvalidTransition
	}
	if skill.Status != "published" {
		return ErrInvalidTransition
	}

	ok, err := l.membership.IsScopeAdmin(ctx, callerID, skill.ScopeID)
	if err != nil {
		return err
	}
	if !ok {
		return ErrForbidden
	}

	now := time.Now().UTC()
	return l.dbOps.updateSkillStatus(ctx, skillID, "deprecated", (*time.Time)(nil), &now)
}

// Republish transitions a deprecated skill back to published; requires scope admin.
func (l *Lifecycle) Republish(ctx context.Context, skillID, callerID uuid.UUID) error {
	skill, err := l.dbOps.getSkill(ctx, skillID)
	if err != nil {
		return err
	}
	if skill == nil {
		return ErrInvalidTransition
	}
	if skill.Status != "deprecated" {
		return ErrInvalidTransition
	}

	ok, err := l.membership.IsScopeAdmin(ctx, callerID, skill.ScopeID)
	if err != nil {
		return err
	}
	if !ok {
		return ErrForbidden
	}

	now := time.Now().UTC()
	return l.dbOps.updateSkillStatus(ctx, skillID, "published", &now, (*time.Time)(nil))
}

// EmergencyRollback transitions any non-draft skill back to draft; requires scope admin.
func (l *Lifecycle) EmergencyRollback(ctx context.Context, skillID, callerID uuid.UUID) error {
	skill, err := l.dbOps.getSkill(ctx, skillID)
	if err != nil {
		return err
	}
	if skill == nil {
		return ErrInvalidTransition
	}
	if skill.Status == "draft" {
		return ErrInvalidTransition
	}

	ok, err := l.membership.IsScopeAdmin(ctx, callerID, skill.ScopeID)
	if err != nil {
		return err
	}
	if !ok {
		return ErrForbidden
	}

	return l.dbOps.updateSkillStatus(ctx, skillID, "draft", (*time.Time)(nil), (*time.Time)(nil))
}
