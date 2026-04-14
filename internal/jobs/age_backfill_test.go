package jobs

import (
	"context"
	"fmt"
	"testing"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"

	"github.com/simplyblock/postbrain/internal/graph"
)

func TestShouldSkipAGEBackfillRelationSyncError_AGEUnavailable(t *testing.T) {
	if shouldSkipAGEBackfillRelationSyncError(graph.ErrAGEUnavailable) {
		t.Fatal("ErrAGEUnavailable must not be skipped; it triggers a clean mid-run abort instead")
	}
}

func TestShouldSkipAGEBackfillRelationSyncError_EntityUpdateFailureInternalError(t *testing.T) {
	err := fmt.Errorf("graph: sync relation to age: %w", &pgconn.PgError{Code: "XX000", Message: "Entity failed to be updated: 3"})
	if !shouldSkipAGEBackfillRelationSyncError(err) {
		t.Fatal("expected entity update internal AGE failure to be skippable")
	}
}

func TestShouldSkipAGEBackfillRelationSyncError_NonMatchingError(t *testing.T) {
	err := fmt.Errorf("graph: sync relation to age: %w", &pgconn.PgError{Code: "23505", Message: "duplicate key value violates unique constraint"})
	if shouldSkipAGEBackfillRelationSyncError(err) {
		t.Fatal("did not expect non-matching error to be skippable")
	}
}

func TestNewAGEBackfillJob_DefaultBatchSize(t *testing.T) {
	j := NewAGEBackfillJob(nil, 0)
	if j.batchSize != 500 {
		t.Fatalf("default batch size = %d, want 500", j.batchSize)
	}
}

func TestNewAGEBackfillJob_CustomBatchSize(t *testing.T) {
	j := NewAGEBackfillJob(nil, 128)
	if j.batchSize != 128 {
		t.Fatalf("custom batch size = %d, want 128", j.batchSize)
	}
}

type fakeAGEBackfillLockConn struct {
	queryRowSQL string
	queryRowCtx context.Context
	queryRowErr error
	queryRowVal bool

	execSQL string
	execErr error
}

func (f *fakeAGEBackfillLockConn) QueryRow(ctx context.Context, sql string, _ ...any) pgx.Row {
	f.queryRowCtx = ctx
	f.queryRowSQL = sql
	if f.queryRowErr != nil {
		return fakeBoolRow{err: f.queryRowErr}
	}
	return fakeBoolRow{val: f.queryRowVal}
}

func (f *fakeAGEBackfillLockConn) Exec(_ context.Context, sql string, _ ...any) (pgconn.CommandTag, error) {
	f.execSQL = sql
	if f.execErr != nil {
		return pgconn.CommandTag{}, f.execErr
	}
	return pgconn.CommandTag{}, nil
}

type fakeBoolRow struct {
	val bool
	err error
}

func (r fakeBoolRow) Scan(dest ...any) error {
	if r.err != nil {
		return r.err
	}
	if len(dest) != 1 {
		return fmt.Errorf("unexpected destination count: %d", len(dest))
	}
	b, ok := dest[0].(*bool)
	if !ok {
		return fmt.Errorf("expected *bool destination")
	}
	*b = r.val
	return nil
}

func TestTryAcquireAGEBackfillLock(t *testing.T) {
	fake := &fakeAGEBackfillLockConn{queryRowVal: true}
	locked, err := tryAcquireAGEBackfillLock(context.Background(), fake)
	if err != nil {
		t.Fatalf("tryAcquireAGEBackfillLock: %v", err)
	}
	if !locked {
		t.Fatal("expected advisory lock to be acquired")
	}
	if fake.queryRowSQL != ageBackfillTryAdvisoryLockSQL {
		t.Fatalf("lock SQL = %q, want %q", fake.queryRowSQL, ageBackfillTryAdvisoryLockSQL)
	}
}

func TestTryAcquireAGEBackfillLock_QueryError(t *testing.T) {
	fake := &fakeAGEBackfillLockConn{queryRowErr: fmt.Errorf("boom")}
	_, err := tryAcquireAGEBackfillLock(context.Background(), fake)
	if err == nil {
		t.Fatal("expected query error")
	}
}

func TestReleaseAGEBackfillLock(t *testing.T) {
	fake := &fakeAGEBackfillLockConn{queryRowVal: true}
	if err := releaseAGEBackfillLock(context.Background(), fake); err != nil {
		t.Fatalf("releaseAGEBackfillLock: %v", err)
	}
	if fake.queryRowSQL != ageBackfillAdvisoryUnlockSQL {
		t.Fatalf("unlock SQL = %q, want %q", fake.queryRowSQL, ageBackfillAdvisoryUnlockSQL)
	}
}

func TestReleaseAGEBackfillLockWithTimeout_UsesDeadlineContext(t *testing.T) {
	fake := &fakeAGEBackfillLockConn{queryRowVal: true}
	if err := releaseAGEBackfillLockWithTimeout(fake); err != nil {
		t.Fatalf("releaseAGEBackfillLockWithTimeout: %v", err)
	}
	if fake.queryRowCtx == nil {
		t.Fatal("expected unlock query context to be captured")
	}
	if _, ok := fake.queryRowCtx.Deadline(); !ok {
		t.Fatal("expected unlock query context to have deadline")
	}
}
