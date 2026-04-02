package oauth

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/simplyblock/postbrain/internal/db"
)

type fakeCodeQueries struct {
	codes map[string]*db.OauthAuthCode
	now   func() time.Time
}

func (f *fakeCodeQueries) IssueCode(_ context.Context, arg db.IssueCodeParams) (*db.OauthAuthCode, error) {
	row := &db.OauthAuthCode{
		ID:            uuid.New(),
		CodeHash:      arg.CodeHash,
		ClientID:      arg.ClientID,
		PrincipalID:   arg.PrincipalID,
		RedirectUri:   arg.RedirectUri,
		Scopes:        arg.Scopes,
		CodeChallenge: arg.CodeChallenge,
		ExpiresAt:     arg.ExpiresAt,
		CreatedAt:     f.now(),
	}
	f.codes[arg.CodeHash] = row
	return row, nil
}

func (f *fakeCodeQueries) ConsumeCode(_ context.Context, codeHash string) (*db.OauthAuthCode, error) {
	row, ok := f.codes[codeHash]
	if !ok {
		return nil, pgx.ErrNoRows
	}
	if row.UsedAt != nil || !row.ExpiresAt.After(f.now()) {
		return nil, pgx.ErrNoRows
	}
	usedAt := f.now()
	row.UsedAt = &usedAt
	return row, nil
}

func (f *fakeCodeQueries) GetCodeByHash(_ context.Context, codeHash string) (*db.OauthAuthCode, error) {
	row, ok := f.codes[codeHash]
	if !ok {
		return nil, pgx.ErrNoRows
	}
	return row, nil
}

func newTestCodeStore(now time.Time) (*CodeStore, *fakeCodeQueries) {
	fake := &fakeCodeQueries{
		codes: map[string]*db.OauthAuthCode{},
		now:   func() time.Time { return now },
	}
	store := &CodeStore{
		q:   fake,
		now: func() time.Time { return now },
	}
	return store, fake
}

func TestCodeStore_Issue_StoresHash_NotRaw(t *testing.T) {
	now := time.Date(2026, 4, 2, 10, 0, 0, 0, time.UTC)
	store, fake := newTestCodeStore(now)
	ctx := context.Background()

	rawCode, err := store.Issue(ctx, IssueCodeRequest{
		ClientID:      uuid.New(),
		PrincipalID:   uuid.New(),
		RedirectURI:   "http://localhost/callback",
		Scopes:        []string{ScopeMemoriesRead},
		CodeChallenge: "challenge",
		TTL:           10 * time.Minute,
	})
	if err != nil {
		t.Fatalf("Issue: %v", err)
	}
	if _, ok := fake.codes[rawCode]; ok {
		t.Fatal("raw code should not be used as storage key")
	}
	if _, ok := fake.codes[hashSHA256Hex(rawCode)]; !ok {
		t.Fatal("hashed code should be used as storage key")
	}
}

func TestCodeStore_Consume_ValidCode_ReturnsRecord(t *testing.T) {
	now := time.Date(2026, 4, 2, 10, 0, 0, 0, time.UTC)
	store, _ := newTestCodeStore(now)
	ctx := context.Background()

	rawCode, err := store.Issue(ctx, IssueCodeRequest{
		ClientID:      uuid.New(),
		PrincipalID:   uuid.New(),
		RedirectURI:   "http://localhost/callback",
		Scopes:        []string{ScopeMemoriesRead},
		CodeChallenge: "challenge",
		TTL:           10 * time.Minute,
	})
	if err != nil {
		t.Fatalf("Issue: %v", err)
	}

	got, err := store.Consume(ctx, rawCode)
	if err != nil {
		t.Fatalf("Consume: %v", err)
	}
	if got == nil || got.CodeChallenge != "challenge" {
		t.Fatalf("Consume returned %+v, want code_challenge=challenge", got)
	}
}

func TestCodeStore_Consume_AlreadyUsed_ReturnsError(t *testing.T) {
	now := time.Date(2026, 4, 2, 10, 0, 0, 0, time.UTC)
	store, _ := newTestCodeStore(now)
	ctx := context.Background()

	rawCode, err := store.Issue(ctx, IssueCodeRequest{
		ClientID:      uuid.New(),
		PrincipalID:   uuid.New(),
		RedirectURI:   "http://localhost/callback",
		Scopes:        []string{ScopeMemoriesRead},
		CodeChallenge: "challenge",
		TTL:           10 * time.Minute,
	})
	if err != nil {
		t.Fatalf("Issue: %v", err)
	}
	if _, err := store.Consume(ctx, rawCode); err != nil {
		t.Fatalf("first Consume: %v", err)
	}
	if _, err := store.Consume(ctx, rawCode); !errors.Is(err, ErrCodeUsed) {
		t.Fatalf("second Consume error = %v, want ErrCodeUsed", err)
	}
}

func TestCodeStore_Consume_Expired_ReturnsError(t *testing.T) {
	now := time.Date(2026, 4, 2, 10, 0, 0, 0, time.UTC)
	store, fake := newTestCodeStore(now)
	ctx := context.Background()

	rawCode := "expired-raw-code"
	fake.codes[hashSHA256Hex(rawCode)] = &db.OauthAuthCode{
		ID:            uuid.New(),
		CodeHash:      hashSHA256Hex(rawCode),
		ClientID:      uuid.New(),
		PrincipalID:   uuid.New(),
		RedirectUri:   "http://localhost/callback",
		Scopes:        []string{ScopeMemoriesRead},
		CodeChallenge: "challenge",
		ExpiresAt:     now.Add(-time.Minute),
		CreatedAt:     now.Add(-2 * time.Minute),
	}

	if _, err := store.Consume(ctx, rawCode); !errors.Is(err, ErrCodeExpired) {
		t.Fatalf("Consume expired error = %v, want ErrCodeExpired", err)
	}
}

func TestCodeStore_VerifyPKCE_ValidVerifier_OK(t *testing.T) {
	store, _ := newTestCodeStore(time.Now())
	verifier := "dBjftJeZ4CVP-mB92K27uhbUJU1p1r_wW1gFWFOEjXk"
	code := &AuthCode{CodeChallenge: GenerateChallenge(verifier)}
	if err := store.VerifyPKCE(code, verifier); err != nil {
		t.Fatalf("VerifyPKCE valid: %v", err)
	}
}

func TestCodeStore_VerifyPKCE_InvalidVerifier_ReturnsError(t *testing.T) {
	store, _ := newTestCodeStore(time.Now())
	code := &AuthCode{CodeChallenge: GenerateChallenge("another-verifier")}
	if err := store.VerifyPKCE(code, "wrong-verifier"); err == nil {
		t.Fatal("VerifyPKCE invalid verifier: expected error, got nil")
	}
}
