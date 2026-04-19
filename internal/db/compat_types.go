package db

import (
	"time"

	"github.com/google/uuid"
)

// SkillFileInput describes one supplementary file to attach to a skill.
// RelativePath must include the typed subdirectory prefix:
//   - "scripts/foo.sh"    for executable scripts (is_executable=true)
//   - "references/bar.md" for additional markdown reference files
type SkillFileInput struct {
	RelativePath string
	Content      string
	IsExecutable bool
}

// SkillParameter is the in-memory representation of one parameter descriptor.
// Not stored in a DB column directly; serialised to/from JSONB.
type SkillParameter struct {
	Name        string   `json:"name"`
	Type        string   `json:"type"` // string | integer | boolean | enum
	Required    bool     `json:"required"`
	Default     any      `json:"default,omitempty"`
	Description string   `json:"description"`
	Values      []string `json:"values,omitempty"` // for enum type
}

// Membership is an alias for PrincipalMembership for backward compatibility.
type Membership = PrincipalMembership

// MembershipRow is a denormalised membership row with principal display names.
type MembershipRow struct {
	MemberID          uuid.UUID
	MemberSlug        string
	MemberDisplayName string
	ParentID          uuid.UUID
	ParentSlug        string
	ParentDisplayName string
	Role              string
	CreatedAt         time.Time
}

// MemoryScore pairs a memory with its retrieval scores.
type MemoryScore struct {
	Memory    *Memory
	VecScore  float64
	BM25Score float64
	TrgmScore float64
}

// ArtifactScore pairs a knowledge artifact with its retrieval scores.
type ArtifactScore struct {
	Artifact  *KnowledgeArtifact
	VecScore  float64
	BM25Score float64
	TrgmScore float64
}

// SkillScore pairs a skill with its retrieval score.
type SkillScore struct {
	Skill *Skill
	Score float64
}

// DigestLog is an audit record for a synthesis operation.
type DigestLog struct {
	ID            uuid.UUID
	ScopeID       uuid.UUID
	DigestID      uuid.UUID
	SourceIDs     []uuid.UUID
	Strategy      string
	SynthesisedBy *uuid.UUID
	CreatedAt     time.Time
}
