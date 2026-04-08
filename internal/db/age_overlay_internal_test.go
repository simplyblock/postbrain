package db

import (
	"context"
	"fmt"
	"testing"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

type fakeAGEOverlayExecutor struct {
	execSQLs    []string
	queryRowSQL []string
	rowVals     []bool
	rowErr      error
}

func (f *fakeAGEOverlayExecutor) Exec(_ context.Context, sql string, _ ...any) (pgconn.CommandTag, error) {
	f.execSQLs = append(f.execSQLs, sql)
	return pgconn.CommandTag{}, nil
}

func (f *fakeAGEOverlayExecutor) QueryRow(_ context.Context, sql string, _ ...any) pgx.Row {
	f.queryRowSQL = append(f.queryRowSQL, sql)
	if f.rowErr != nil {
		return fakeBoolRow{err: f.rowErr}
	}
	if len(f.rowVals) == 0 {
		return fakeBoolRow{err: fmt.Errorf("no fake bool row value configured")}
	}
	v := f.rowVals[0]
	f.rowVals = f.rowVals[1:]
	return fakeBoolRow{val: v}
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

func TestEnsureAGEOverlayOnExecutor_UsesSequentialSteps(t *testing.T) {
	exec := &fakeAGEOverlayExecutor{rowVals: []bool{false}}
	if err := ensureAGEOverlayOnExecutor(context.Background(), exec, "pb_user"); err != nil {
		t.Fatalf("ensureAGEOverlayOnExecutor: %v", err)
	}
	if len(exec.execSQLs) != 2 {
		t.Fatalf("exec call count = %d, want 2 (overlay + privileges)", len(exec.execSQLs))
	}
	if exec.execSQLs[0] != ensureAGEOverlaySQL {
		t.Fatalf("first exec should be ensureAGEOverlaySQL")
	}
	if exec.execSQLs[1] != ensureAGEPrivilegesSQL {
		t.Fatalf("second exec should be ensureAGEPrivilegesSQL")
	}
	if len(exec.queryRowSQL) != 1 {
		t.Fatalf("query row call count = %d, want 1 when age is not installed", len(exec.queryRowSQL))
	}
}

func TestEnsureAGEOverlayOnExecutor_AgeInstalledRunsSchemaChecksAndProbe(t *testing.T) {
	exec := &fakeAGEOverlayExecutor{rowVals: []bool{true, true, true}}
	if err := ensureAGEOverlayOnExecutor(context.Background(), exec, "pb_user"); err != nil {
		t.Fatalf("ensureAGEOverlayOnExecutor: %v", err)
	}
	if len(exec.queryRowSQL) != 3 {
		t.Fatalf("query row call count = %d, want 3", len(exec.queryRowSQL))
	}
	if got := exec.queryRowSQL[0]; got != "SELECT EXISTS (SELECT 1 FROM pg_extension WHERE extname='age')" {
		t.Fatalf("unexpected extension detect query: %q", got)
	}
	if got := exec.queryRowSQL[1]; got != "SELECT EXISTS (SELECT 1 FROM pg_namespace WHERE nspname='postbrain')" {
		t.Fatalf("unexpected graph schema detect query: %q", got)
	}
	if got := exec.queryRowSQL[2]; got != "SELECT has_schema_privilege(current_user, 'postbrain', 'USAGE')" {
		t.Fatalf("unexpected schema usage query: %q", got)
	}
	if len(exec.execSQLs) != 3 {
		t.Fatalf("exec call count = %d, want 3 (overlay + privileges + probe)", len(exec.execSQLs))
	}
	if exec.execSQLs[2] != ensureAGEAccessProbeSQL {
		t.Fatalf("third exec should be ensureAGEAccessProbeSQL")
	}
}
