package oauth

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/simplyblock/postbrain/internal/db"
)

var (
	ErrCodeNotFound     = errors.New("oauth: authorization code not found")
	ErrCodeUsed         = errors.New("oauth: authorization code already used")
	ErrCodeExpired      = errors.New("oauth: authorization code expired")
	ErrPKCEMismatch     = errors.New("oauth: pkce verifier mismatch")
	ErrMissingChallenge = errors.New("oauth: missing code challenge")
)

type codeQueries interface {
	IssueCode(ctx context.Context, arg db.IssueCodeParams) (*db.OauthAuthCode, error)
	ConsumeCode(ctx context.Context, codeHash string) (*db.OauthAuthCode, error)
	GetCodeByHash(ctx context.Context, codeHash string) (*db.OauthAuthCode, error)
}

// IssueCodeRequest contains data needed to create an auth code.
type IssueCodeRequest struct {
	ClientID      uuid.UUID
	PrincipalID   uuid.UUID
	RedirectURI   string
	Scopes        []string
	CodeChallenge string
	TTL           time.Duration
}

// AuthCode is the domain model for oauth_auth_codes rows.
type AuthCode struct {
	ID            uuid.UUID
	CodeHash      string
	ClientID      uuid.UUID
	PrincipalID   uuid.UUID
	RedirectURI   string
	Scopes        []string
	CodeChallenge string
	ExpiresAt     time.Time
	UsedAt        *time.Time
	CreatedAt     time.Time
}

// CodeStore issues and consumes authorization codes.
type CodeStore struct {
	q   codeQueries
	now func() time.Time
}

// NewCodeStore constructs a CodeStore backed by sqlc queries.
func NewCodeStore(pool *pgxpool.Pool) *CodeStore {
	return &CodeStore{
		q:   db.New(pool),
		now: time.Now,
	}
}

// Issue creates a new authorization code and stores only its hash.
func (s *CodeStore) Issue(ctx context.Context, req IssueCodeRequest) (string, error) {
	rawCode, err := generateRawCode()
	if err != nil {
		return "", err
	}
	codeHash := hashSHA256Hex(rawCode)

	_, err = s.q.IssueCode(ctx, db.IssueCodeParams{
		CodeHash:      codeHash,
		ClientID:      req.ClientID,
		PrincipalID:   req.PrincipalID,
		RedirectUri:   req.RedirectURI,
		Scopes:        req.Scopes,
		CodeChallenge: req.CodeChallenge,
		ExpiresAt:     s.now().Add(req.TTL),
	})
	if err != nil {
		return "", err
	}
	return rawCode, nil
}

// Consume marks an authorization code as used atomically.
func (s *CodeStore) Consume(ctx context.Context, rawCode string) (*AuthCode, error) {
	codeHash := hashSHA256Hex(rawCode)
	row, err := s.q.ConsumeCode(ctx, codeHash)
	if err == nil {
		return toAuthCode(row), nil
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return nil, err
	}

	lookedUp, lookupErr := s.q.GetCodeByHash(ctx, codeHash)
	if lookupErr != nil {
		if errors.Is(lookupErr, pgx.ErrNoRows) {
			return nil, ErrCodeNotFound
		}
		return nil, lookupErr
	}
	if lookedUp.UsedAt != nil {
		return nil, ErrCodeUsed
	}
	if !lookedUp.ExpiresAt.After(s.now()) {
		return nil, ErrCodeExpired
	}
	return nil, ErrCodeNotFound
}

// VerifyPKCE validates an S256 verifier against a stored challenge.
func (s *CodeStore) VerifyPKCE(code *AuthCode, verifier string) error {
	if code == nil || code.CodeChallenge == "" {
		return ErrMissingChallenge
	}
	if !VerifyS256(verifier, code.CodeChallenge) {
		return ErrPKCEMismatch
	}
	return nil
}

func toAuthCode(row *db.OauthAuthCode) *AuthCode {
	if row == nil {
		return nil
	}
	return &AuthCode{
		ID:            row.ID,
		CodeHash:      row.CodeHash,
		ClientID:      row.ClientID,
		PrincipalID:   row.PrincipalID,
		RedirectURI:   row.RedirectUri,
		Scopes:        row.Scopes,
		CodeChallenge: row.CodeChallenge,
		ExpiresAt:     row.ExpiresAt,
		UsedAt:        row.UsedAt,
		CreatedAt:     row.CreatedAt,
	}
}

func generateRawCode() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", fmt.Errorf("generate code: %w", err)
	}
	return base64.RawURLEncoding.EncodeToString(b), nil
}
