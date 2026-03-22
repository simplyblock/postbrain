// Package graph provides entity and relation management for the Postbrain
// knowledge graph layer.
package graph

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"strings"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/simplyblock/postbrain/internal/db"
)

// ErrInvalidRole is returned when a role other than the four valid values is supplied.
var ErrInvalidRole = errors.New("graph: role must be one of subject, object, context, related")

var validRoles = map[string]bool{
	"subject": true,
	"object":  true,
	"context": true,
	"related": true,
}

// Store provides entity and relation operations.
type Store struct {
	pool *pgxpool.Pool
}

// NewStore creates a new Store backed by the given pool.
func NewStore(pool *pgxpool.Pool) *Store {
	return &Store{pool: pool}
}

// UpsertEntity inserts or updates an entity by (scope_id, entity_type, canonical).
func (s *Store) UpsertEntity(ctx context.Context, scopeID uuid.UUID, entityType, name, canonical string, meta []byte) (*db.Entity, error) {
	e := &db.Entity{
		ScopeID:    scopeID,
		EntityType: entityType,
		Name:       name,
		Canonical:  canonical,
		Meta:       meta,
	}
	result, err := db.UpsertEntity(ctx, s.pool, e)
	if err != nil {
		return nil, fmt.Errorf("graph: upsert entity: %w", err)
	}
	return result, nil
}

// GetEntityByCanonical looks up an entity by scope, type, and canonical name.
func (s *Store) GetEntityByCanonical(ctx context.Context, scopeID uuid.UUID, entityType, canonical string) (*db.Entity, error) {
	result, err := db.GetEntityByCanonical(ctx, s.pool, scopeID, entityType, canonical)
	if err != nil {
		return nil, fmt.Errorf("graph: get entity: %w", err)
	}
	return result, nil
}

// UpsertRelation inserts or updates a relation between two entities.
func (s *Store) UpsertRelation(ctx context.Context, scopeID, subjectID uuid.UUID, predicate string, objectID uuid.UUID, confidence float64, sourceMemoryID *uuid.UUID) (*db.Relation, error) {
	r := &db.Relation{
		ScopeID:      scopeID,
		SubjectID:    subjectID,
		Predicate:    predicate,
		ObjectID:     objectID,
		Confidence:   confidence,
		SourceMemory: sourceMemoryID,
	}
	result, err := db.UpsertRelation(ctx, s.pool, r)
	if err != nil {
		return nil, fmt.Errorf("graph: upsert relation: %w", err)
	}
	return result, nil
}

// LinkMemoryToEntity links a memory to an entity with a role.
// Valid roles: subject, object, context, related.
func (s *Store) LinkMemoryToEntity(ctx context.Context, memoryID, entityID uuid.UUID, role string) error {
	if !validRoles[role] {
		return ErrInvalidRole
	}
	err := db.LinkMemoryToEntity(ctx, s.pool, memoryID, entityID, role)
	if err != nil {
		return fmt.Errorf("graph: link memory to entity: %w", err)
	}
	return nil
}

// ListRelationsForEntity returns relations where the entity is subject or object.
func (s *Store) ListRelationsForEntity(ctx context.Context, entityID uuid.UUID, predicate string) ([]*db.Relation, error) {
	result, err := db.ListRelationsForEntity(ctx, s.pool, entityID, predicate)
	if err != nil {
		return nil, fmt.Errorf("graph: list relations: %w", err)
	}
	return result, nil
}

// prPattern matches "pr:NNN" patterns.
var prPattern = regexp.MustCompile(`\bpr:\d+\b`)

// pascalPattern matches PascalCase words (two or more words capitalised).
// Matches words starting with an uppercase letter followed by at least one lowercase,
// then at least one uppercase letter (e.g. UserRepository, AuthService).
// All-caps abbreviations (e.g. UUID) are excluded by requiring at least one lowercase.
var pascalPattern = regexp.MustCompile(`\b[A-Z][a-z]+[A-Za-z]*[A-Z][A-Za-z]*\b`)

// ExtractEntitiesFromMemory extracts entity candidates from memory content and source_ref.
//
// Heuristics:
//   - If sourceRef starts with "file:": extract the file path as a "file" entity.
//   - Extract "pr:NNN" patterns as "pr" entities.
//   - Extract PascalCase words from content as "concept" entities (limit 10 unique).
func ExtractEntitiesFromMemory(content string, sourceRef *string) []*db.Entity {
	var entities []*db.Entity
	seen := make(map[string]bool)

	// File entity from sourceRef.
	if sourceRef != nil && strings.HasPrefix(*sourceRef, "file:") {
		rest := (*sourceRef)[len("file:"):]
		// Strip trailing :<line>.
		if idx := strings.LastIndex(rest, ":"); idx != -1 {
			rest = rest[:idx]
		}
		if rest != "" && !seen[rest] {
			seen[rest] = true
			entities = append(entities, &db.Entity{
				EntityType: "file",
				Name:       rest,
				Canonical:  rest,
			})
		}
	}

	// PR entities from content.
	for _, match := range prPattern.FindAllString(content, -1) {
		if !seen[match] {
			seen[match] = true
			entities = append(entities, &db.Entity{
				EntityType: "pr",
				Name:       match,
				Canonical:  match,
			})
		}
	}

	// Concept entities from PascalCase words.
	conceptCount := 0
	for _, match := range pascalPattern.FindAllString(content, -1) {
		if conceptCount >= 10 {
			break
		}
		if !seen[match] {
			seen[match] = true
			entities = append(entities, &db.Entity{
				EntityType: "concept",
				Name:       match,
				Canonical:  match,
			})
			conceptCount++
		}
	}

	return entities
}
