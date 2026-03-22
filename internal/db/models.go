package db

import (
	"time"

	"github.com/google/uuid"
)

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
