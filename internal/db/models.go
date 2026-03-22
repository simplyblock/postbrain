package db

import (
	"time"

	"github.com/google/uuid"
)

// Skill represents a versioned, parameterised prompt template stored in the registry.
type Skill struct {
	ID               uuid.UUID
	ScopeID          uuid.UUID
	AuthorID         uuid.UUID
	SourceArtifactID *uuid.UUID
	Slug             string
	Name             string
	Description      string
	AgentTypes       []string
	Body             string
	Parameters       []byte // JSONB: [{name,type,required,default,description,values?}]
	Visibility       string
	Status           string
	PublishedAt      *time.Time
	DeprecatedAt     *time.Time
	ReviewRequired   int
	Version          int
	PreviousVersion  *uuid.UUID
	Embedding        []float32
	EmbeddingModelID *uuid.UUID
	InvocationCount  int
	LastInvokedAt    *time.Time
	CreatedAt        time.Time
	UpdatedAt        time.Time
}

// SkillEndorsement records a single endorsement of a skill by a principal.
type SkillEndorsement struct {
	ID         uuid.UUID
	SkillID    uuid.UUID
	EndorserID uuid.UUID
	Note       *string
	CreatedAt  time.Time
}

// SkillHistory captures a point-in-time snapshot of a skill's body and parameters.
type SkillHistory struct {
	ID         uuid.UUID
	SkillID    uuid.UUID
	Version    int
	Body       string
	Parameters []byte
	ChangedBy  uuid.UUID
	ChangeNote *string
	CreatedAt  time.Time
}

// SkillParameter is the in-memory representation of one parameter descriptor.
type SkillParameter struct {
	Name        string   `json:"name"`
	Type        string   `json:"type"` // string | integer | boolean | enum
	Required    bool     `json:"required"`
	Default     any      `json:"default,omitempty"`
	Description string   `json:"description"`
	Values      []string `json:"values,omitempty"` // for enum type
}

// Principal represents an actor in the system (agent, user, team, department, or company).
type Principal struct {
	ID          uuid.UUID
	Kind        string // "agent" | "user" | "team" | "department" | "company"
	Slug        string
	DisplayName string
	Meta        []byte // JSONB as raw bytes
	CreatedAt   time.Time
	UpdatedAt   time.Time
}

// Membership represents a principal's membership within a parent principal.
type Membership struct {
	MemberID  uuid.UUID
	ParentID  uuid.UUID
	Role      string
	GrantedBy *uuid.UUID
	CreatedAt time.Time
}

// Scope represents a namespace for memory and knowledge, forming a hierarchy via ltree.
type Scope struct {
	ID          uuid.UUID
	Kind        string
	ExternalID  string
	Name        string
	ParentID    *uuid.UUID
	PrincipalID uuid.UUID
	Path        string // ltree as string
	Meta        []byte
	CreatedAt   time.Time
}

// Memory represents a single captured memory record.
// TODO(task-memory): expand this struct when the memory package is implemented.
type Memory struct {
	ID              uuid.UUID
	MemoryType      string
	ScopeID         uuid.UUID
	AuthorID        uuid.UUID
	Content         string
	Summary         *string
	PromotionStatus string
	PromotedTo      *uuid.UUID
	SourceRef       *string
	CreatedAt       time.Time
	UpdatedAt       time.Time
}

// KnowledgeArtifact represents a curated, versioned knowledge artifact.
type KnowledgeArtifact struct {
	ID               uuid.UUID
	KnowledgeType    string // semantic|episodic|procedural|reference
	OwnerScopeID     uuid.UUID
	AuthorID         uuid.UUID
	Visibility       string // private|project|team|department|company
	Status           string // draft|in_review|published|deprecated
	PublishedAt      *time.Time
	DeprecatedAt     *time.Time
	ReviewRequired   int
	Title            string
	Content          string
	Summary          *string
	Embedding        []float32
	EmbeddingModelID *uuid.UUID
	Meta             []byte
	EndorsementCount int
	AccessCount      int
	LastAccessed     *time.Time
	Version          int
	PreviousVersion  *uuid.UUID
	SourceMemoryID   *uuid.UUID
	SourceRef        *string
	CreatedAt        time.Time
	UpdatedAt        time.Time
}

// KnowledgeEndorsement records a single endorsement of a knowledge artifact by a principal.
type KnowledgeEndorsement struct {
	ID         uuid.UUID
	ArtifactID uuid.UUID
	EndorserID uuid.UUID
	Note       *string
	CreatedAt  time.Time
}

// KnowledgeHistory captures a point-in-time snapshot of a knowledge artifact's content.
type KnowledgeHistory struct {
	ID         uuid.UUID
	ArtifactID uuid.UUID
	Version    int
	Content    string
	Summary    *string
	ChangedBy  uuid.UUID
	ChangeNote *string
	CreatedAt  time.Time
}

// KnowledgeCollection is a curated bundle of knowledge artifacts.
type KnowledgeCollection struct {
	ID          uuid.UUID
	ScopeID     uuid.UUID
	OwnerID     uuid.UUID
	Slug        string
	Name        string
	Description *string
	Visibility  string
	Meta        []byte
	CreatedAt   time.Time
	UpdatedAt   time.Time
}

// KnowledgeCollectionItem is a membership record linking an artifact to a collection.
type KnowledgeCollectionItem struct {
	CollectionID uuid.UUID
	ArtifactID   uuid.UUID
	Position     int
	AddedBy      uuid.UUID
	AddedAt      time.Time
}

// StalenessFlag records a signal that a published knowledge artifact may be stale.
type StalenessFlag struct {
	ID         uuid.UUID
	ArtifactID uuid.UUID
	Signal     string // source_modified|contradiction_detected|low_access_age
	Confidence float64
	Evidence   []byte
	Status     string // open|dismissed|resolved
	FlaggedAt  time.Time
	ReviewedBy *uuid.UUID
	ReviewedAt *time.Time
	ReviewNote *string
}

// PromotionRequest represents a request to promote a memory into a knowledge artifact.
type PromotionRequest struct {
	ID                   uuid.UUID
	MemoryID             uuid.UUID
	RequestedBy          uuid.UUID
	TargetScopeID        uuid.UUID
	TargetVisibility     string
	ProposedTitle        *string
	ProposedCollectionID *uuid.UUID
	Status               string // pending|approved|rejected|merged
	ReviewerID           *uuid.UUID
	ReviewNote           *string
	ReviewedAt           *time.Time
	ResultArtifactID     *uuid.UUID
	CreatedAt            time.Time
}

// Token represents an API bearer token (raw value never stored; only the SHA-256 hash).
type Token struct {
	ID          uuid.UUID
	PrincipalID uuid.UUID
	TokenHash   string
	Name        string
	ScopeIDs    []uuid.UUID // nil = all scopes
	Permissions []string
	ExpiresAt   *time.Time
	LastUsedAt  *time.Time
	CreatedAt   time.Time
	RevokedAt   *time.Time
}
