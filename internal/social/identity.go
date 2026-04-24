package social

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/simplyblock/postbrain/internal/db"
)

// IdentityStore links external provider identities to local principals.
type IdentityStore struct {
	pool *pgxpool.Pool
}

var ErrPrincipalNotProvisioned = errors.New("social: principal not pre-provisioned")

// IdentityPolicy controls whether missing social users may be auto-created.
type IdentityPolicy struct {
	AutoCreateUsers bool
}

func NewIdentityStore(pool *pgxpool.Pool) *IdentityStore {
	return &IdentityStore{pool: pool}
}

// FindOrCreate resolves an existing social identity or creates a new user principal.
func (s *IdentityStore) FindOrCreate(ctx context.Context, provider string, info *UserInfo) (*db.Principal, error) {
	return s.FindOrCreateWithPolicy(ctx, provider, info, IdentityPolicy{AutoCreateUsers: true})
}

// FindOrCreateWithPolicy resolves an existing social identity and either auto-creates
// missing principals or links only pre-provisioned principals based on policy.
func (s *IdentityStore) FindOrCreateWithPolicy(ctx context.Context, provider string, info *UserInfo, policy IdentityPolicy) (*db.Principal, error) {
	tx, err := s.pool.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.Serializable})
	if err != nil {
		return nil, err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	q := db.New(tx)

	principalID, err := q.FindPrincipalBySocialIdentity(ctx, db.FindPrincipalBySocialIdentityParams{
		Provider:   provider,
		ProviderID: info.ProviderID,
	})
	if err == nil {
		if _, err := q.UpsertSocialIdentity(ctx, db.UpsertSocialIdentityParams{
			PrincipalID: principalID,
			Provider:    provider,
			ProviderID:  info.ProviderID,
			Email:       optionalString(info.Email),
			DisplayName: optionalString(info.DisplayName),
			AvatarUrl:   optionalString(info.AvatarURL),
			RawProfile:  info.RawProfile,
		}); err != nil {
			return nil, err
		}
		principal, err := q.GetPrincipalByID(ctx, principalID)
		if err != nil {
			return nil, err
		}
		if err := tx.Commit(ctx); err != nil {
			return nil, err
		}
		return principal, nil
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return nil, err
	}

	baseSlug := principalSlug(provider, info)
	displayName := strings.TrimSpace(info.DisplayName)
	if displayName == "" {
		displayName = baseSlug
	}

	linkPrincipal := func(principal *db.Principal) (*db.Principal, error) {
		if _, err := q.UpsertSocialIdentity(ctx, db.UpsertSocialIdentityParams{
			PrincipalID: principal.ID,
			Provider:    provider,
			ProviderID:  info.ProviderID,
			Email:       optionalString(info.Email),
			DisplayName: optionalString(info.DisplayName),
			AvatarUrl:   optionalString(info.AvatarURL),
			RawProfile:  info.RawProfile,
		}); err != nil {
			return nil, err
		}
		if err := tx.Commit(ctx); err != nil {
			return nil, err
		}
		return principal, nil
	}

	emailSlug := strings.TrimSpace(info.Email)
	if emailSlug != "" {
		principal, lookupErr := q.GetPrincipalBySlug(ctx, emailSlug)
		if lookupErr == nil {
			if principal.Kind != "user" {
				return nil, fmt.Errorf("email slug %q matched non-user principal kind %q", emailSlug, principal.Kind)
			}
			return linkPrincipal(principal)
		}
		if !errors.Is(lookupErr, pgx.ErrNoRows) {
			return nil, fmt.Errorf("lookup principal by email slug: %w", lookupErr)
		}
	}

	if !policy.AutoCreateUsers {
		return nil, ErrPrincipalNotProvisioned
	}

	slug := baseSlug
	if _, slugErr := q.GetPrincipalBySlug(ctx, baseSlug); slugErr == nil {
		slug = baseSlug + "-" + info.ProviderID
	} else if !errors.Is(slugErr, pgx.ErrNoRows) {
		return nil, slugErr
	}

	principal, err := q.CreatePrincipal(ctx, db.CreatePrincipalParams{
		Kind:        "user",
		Slug:        slug,
		DisplayName: displayName,
		Meta:        []byte(`{}`),
	})
	if err != nil {
		return nil, err
	}

	return linkPrincipal(principal)
}

func principalSlug(provider string, info *UserInfo) string {
	if email := strings.TrimSpace(info.Email); email != "" {
		return email
	}
	if providerID := strings.TrimSpace(info.ProviderID); providerID != "" {
		return provider + "-" + providerID
	}
	return provider + "-user"
}

func optionalString(s string) *string {
	v := strings.TrimSpace(s)
	if v == "" {
		return nil
	}
	return &v
}
