//go:build integration

package db_test

import (
	"context"
	"testing"
	"time"

	"github.com/simplyblock/postbrain/internal/db"
	"github.com/simplyblock/postbrain/internal/testhelper"
)

// TestEndSession_EndedAtIsNeverZero is a regression test for the bug where
// EndSession returned ended_at = "0001-01-01T00:53:28+00:53" (the Go zero time).
//
// Root cause: EndSessionParams.Column2 is time.Time (not *time.Time), so leaving
// it unset sent the Go zero value to Postgres. COALESCE($2::timestamptz, now())
// treated it as a valid non-NULL timestamp and stored it instead of now().
// Fix: compat.EndSession now passes time.Now().UTC() explicitly.
func TestEndSession_EndedAtIsNeverZero(t *testing.T) {
	pool := testhelper.NewTestPool(t)
	ctx := context.Background()

	principal := testhelper.CreateTestPrincipal(t, pool, "user", "session-endedat-user")
	scope := testhelper.CreateTestScope(t, pool, "project", "session-endedat-scope", nil, principal.ID)

	before := time.Now().Add(-time.Second)

	session, err := db.CreateSession(ctx, pool, scope.ID, principal.ID, nil)
	if err != nil {
		t.Fatalf("CreateSession: %v", err)
	}

	ended, err := db.EndSession(ctx, pool, session.ID, nil)
	if err != nil {
		t.Fatalf("EndSession: %v", err)
	}

	if ended.EndedAt == nil {
		t.Fatal("ended_at is nil; expected a non-nil timestamp")
	}

	zero := time.Time{}
	if ended.EndedAt.Equal(zero) {
		t.Fatal("ended_at is the zero time (0001-01-01); COALESCE fix not working")
	}

	after := time.Now().Add(time.Second)
	if ended.EndedAt.Before(before) || ended.EndedAt.After(after) {
		t.Errorf("ended_at = %v; expected a value between %v and %v", ended.EndedAt, before, after)
	}
}
