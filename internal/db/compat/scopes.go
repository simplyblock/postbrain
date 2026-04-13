package compat

import (
	"context"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/simplyblock/postbrain/internal/db"
)

// CreateScope inserts a new scope row.
func CreateScope(ctx context.Context, pool *pgxpool.Pool, kind, externalID, name string, parentID *uuid.UUID, principalID uuid.UUID, meta []byte) (*db.Scope, error) {
	if meta == nil {
		meta = []byte("{}")
	}
	q := db.New(pool)
	row, err := q.CreateScope(ctx, db.CreateScopeParams{
		Kind:        kind,
		ExternalID:  externalID,
		Name:        name,
		Column4:     parentID,
		PrincipalID: principalID,
		Meta:        meta,
	})
	if err != nil {
		return nil, err
	}
	return &db.Scope{
		ID:                row.ID,
		Kind:              row.Kind,
		ExternalID:        row.ExternalID,
		Name:              row.Name,
		ParentID:          row.ParentID,
		PrincipalID:       row.PrincipalID,
		Path:              row.Path,
		Meta:              row.Meta,
		RepoUrl:           row.RepoUrl,
		RepoDefaultBranch: row.RepoDefaultBranch,
		LastIndexedCommit: row.LastIndexedCommit,
		CreatedAt:         row.CreatedAt,
	}, nil
}

// GetScopeByID retrieves a scope by UUID. Returns nil, nil if not found.
func GetScopeByID(ctx context.Context, pool *pgxpool.Pool, id uuid.UUID) (*db.Scope, error) {
	q := db.New(pool)
	row, err := q.GetScopeByID(ctx, id)
	if err == pgx.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &db.Scope{
		ID:                row.ID,
		Kind:              row.Kind,
		ExternalID:        row.ExternalID,
		Name:              row.Name,
		ParentID:          row.ParentID,
		PrincipalID:       row.PrincipalID,
		Path:              row.Path,
		Meta:              row.Meta,
		RepoUrl:           row.RepoUrl,
		RepoDefaultBranch: row.RepoDefaultBranch,
		LastIndexedCommit: row.LastIndexedCommit,
		CreatedAt:         row.CreatedAt,
	}, nil
}

// GetScopeByExternalID retrieves a scope by kind and external_id. Returns nil, nil if not found.
func GetScopeByExternalID(ctx context.Context, pool *pgxpool.Pool, kind, externalID string) (*db.Scope, error) {
	q := db.New(pool)
	row, err := q.GetScopeByExternalID(ctx, db.GetScopeByExternalIDParams{
		Kind:       kind,
		ExternalID: externalID,
	})
	if err == pgx.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &db.Scope{
		ID:                row.ID,
		Kind:              row.Kind,
		ExternalID:        row.ExternalID,
		Name:              row.Name,
		ParentID:          row.ParentID,
		PrincipalID:       row.PrincipalID,
		Path:              row.Path,
		Meta:              row.Meta,
		RepoUrl:           row.RepoUrl,
		RepoDefaultBranch: row.RepoDefaultBranch,
		LastIndexedCommit: row.LastIndexedCommit,
		CreatedAt:         row.CreatedAt,
	}, nil
}

// GetAncestorScopeIDs returns all ancestor scope IDs.
func GetAncestorScopeIDs(ctx context.Context, pool *pgxpool.Pool, scopeID uuid.UUID) ([]uuid.UUID, error) {
	q := db.New(pool)
	return q.GetAncestorScopeIDs(ctx, scopeID)
}

// ListScopes returns scopes ordered by creation time, with pagination.
func ListScopes(ctx context.Context, pool *pgxpool.Pool, limit, offset int) ([]*db.Scope, error) {
	q := db.New(pool)
	rows, err := q.ListScopes(ctx, db.ListScopesParams{Limit: int32(limit), Offset: int32(offset)})
	if err != nil {
		return nil, err
	}
	out := make([]*db.Scope, len(rows))
	for i, row := range rows {
		out[i] = &db.Scope{
			ID:                row.ID,
			Kind:              row.Kind,
			ExternalID:        row.ExternalID,
			Name:              row.Name,
			ParentID:          row.ParentID,
			PrincipalID:       row.PrincipalID,
			Path:              row.Path,
			Meta:              row.Meta,
			RepoUrl:           row.RepoUrl,
			RepoDefaultBranch: row.RepoDefaultBranch,
			LastIndexedCommit: row.LastIndexedCommit,
			CreatedAt:         row.CreatedAt,
		}
	}
	return out, nil
}

// UpdateScope updates the name and meta of a scope.
func UpdateScope(ctx context.Context, pool *pgxpool.Pool, id uuid.UUID, name string, meta []byte) (*db.Scope, error) {
	if meta == nil {
		meta = []byte("{}")
	}
	q := db.New(pool)
	row, err := q.UpdateScope(ctx, db.UpdateScopeParams{ID: id, Name: name, Meta: meta})
	if err == pgx.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &db.Scope{
		ID:                row.ID,
		Kind:              row.Kind,
		ExternalID:        row.ExternalID,
		Name:              row.Name,
		ParentID:          row.ParentID,
		PrincipalID:       row.PrincipalID,
		Path:              row.Path,
		Meta:              row.Meta,
		RepoUrl:           row.RepoUrl,
		RepoDefaultBranch: row.RepoDefaultBranch,
		LastIndexedCommit: row.LastIndexedCommit,
		CreatedAt:         row.CreatedAt,
	}, nil
}

// UpdateScopeOwner updates the owner principal of a scope.
func UpdateScopeOwner(ctx context.Context, pool *pgxpool.Pool, id, principalID uuid.UUID) (*db.Scope, error) {
	q := db.New(pool)
	row, err := q.UpdateScopeOwner(ctx, db.UpdateScopeOwnerParams{ID: id, PrincipalID: principalID})
	if err == pgx.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &db.Scope{
		ID:                row.ID,
		Kind:              row.Kind,
		ExternalID:        row.ExternalID,
		Name:              row.Name,
		ParentID:          row.ParentID,
		PrincipalID:       row.PrincipalID,
		Path:              row.Path,
		Meta:              row.Meta,
		RepoUrl:           row.RepoUrl,
		RepoDefaultBranch: row.RepoDefaultBranch,
		LastIndexedCommit: row.LastIndexedCommit,
		CreatedAt:         row.CreatedAt,
	}, nil
}

// CountChildScopes returns the number of direct child scopes.
func CountChildScopes(ctx context.Context, pool *pgxpool.Pool, id uuid.UUID) (int64, error) {
	q := db.New(pool)
	return q.CountChildScopes(ctx, &id)
}

// DeleteScope removes a scope by UUID.
func DeleteScope(ctx context.Context, pool *pgxpool.Pool, id uuid.UUID) error {
	q := db.New(pool)
	return q.DeleteScope(ctx, id)
}

// SetScopeRepo attaches a git repository URL and default branch to a project-kind scope.
func SetScopeRepo(ctx context.Context, pool *pgxpool.Pool, id uuid.UUID, repoURL, defaultBranch string) (*db.Scope, error) {
	q := db.New(pool)
	row, err := q.SetScopeRepo(ctx, db.SetScopeRepoParams{
		ID:                id,
		RepoUrl:           &repoURL,
		RepoDefaultBranch: defaultBranch,
	})
	if err != nil {
		return nil, fmt.Errorf("db: set scope repo: %w", err)
	}
	return &db.Scope{
		ID:                id,
		Kind:              row.Kind,
		ExternalID:        row.ExternalID,
		Name:              row.Name,
		ParentID:          row.ParentID,
		PrincipalID:       row.PrincipalID,
		Path:              row.Path,
		Meta:              row.Meta,
		RepoUrl:           row.RepoUrl,
		RepoDefaultBranch: row.RepoDefaultBranch,
		LastIndexedCommit: row.LastIndexedCommit,
		CreatedAt:         row.CreatedAt,
	}, nil
}

// SetLastIndexedCommit records the last successfully indexed commit SHA for a scope.
func SetLastIndexedCommit(ctx context.Context, pool *pgxpool.Pool, id uuid.UUID, sha string) error {
	q := db.New(pool)
	return q.SetLastIndexedCommit(ctx, db.SetLastIndexedCommitParams{
		ID:                id,
		LastIndexedCommit: &sha,
	})
}

// GetScopesByIDs fetches all scopes whose IDs are in the provided slice.
// Returns an empty slice (not nil) when ids is empty.
func GetScopesByIDs(ctx context.Context, pool *pgxpool.Pool, ids []uuid.UUID) ([]*db.Scope, error) {
	if len(ids) == 0 {
		return []*db.Scope{}, nil
	}
	rows, err := pool.Query(ctx,
		`SELECT id, kind, external_id, name, parent_id, principal_id, path::text, meta,
		        repo_url, repo_default_branch, last_indexed_commit, created_at
		 FROM scopes WHERE id = ANY($1)
		 ORDER BY created_at DESC`,
		ids,
	)
	if err != nil {
		return nil, fmt.Errorf("db: get scopes by ids: %w", err)
	}
	defer rows.Close()
	var scopes []*db.Scope
	for rows.Next() {
		var s db.Scope
		if err := rows.Scan(&s.ID, &s.Kind, &s.ExternalID, &s.Name, &s.ParentID,
			&s.PrincipalID, &s.Path, &s.Meta, &s.RepoUrl, &s.RepoDefaultBranch, &s.LastIndexedCommit, &s.CreatedAt); err != nil {
			return nil, fmt.Errorf("db: get scopes by ids scan: %w", err)
		}
		scopes = append(scopes, &s)
	}
	return scopes, rows.Err()
}

// ScopeInLineage reports whether scopeA and scopeB are in the same ltree
// lineage (one is an ancestor or descendant of the other, or they are equal).
func ScopeInLineage(ctx context.Context, pool *pgxpool.Pool, scopeA, scopeB uuid.UUID) (bool, error) {
	if scopeA == scopeB {
		return true, nil
	}
	var ok bool
	err := pool.QueryRow(ctx, `
		SELECT EXISTS (
		    SELECT 1 FROM scopes a, scopes b
		    WHERE a.id = $1 AND b.id = $2
		      AND (a.path @> b.path OR b.path @> a.path)
		)`, scopeA, scopeB).Scan(&ok)
	if err != nil {
		return false, fmt.Errorf("db: scope lineage check: %w", err)
	}
	return ok, nil
}
