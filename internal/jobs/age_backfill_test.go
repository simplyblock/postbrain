package jobs

import (
	"strings"
	"testing"
)

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

func TestAGEBackfillJob_Signature(t *testing.T) {
	var _ = (*AGEBackfillJob)(nil).Run
}

func TestEntityBatchQuery_UsesKeysetWhenCursorPresent(t *testing.T) {
	q := entityBatchQuery(true)
	if !strings.Contains(q, "WHERE (created_at, id) > ($1, $2)") {
		t.Fatalf("entity keyset query missing cursor predicate:\n%s", q)
	}
	if !strings.Contains(q, "LIMIT $3") {
		t.Fatalf("entity keyset query missing limit placeholder $3:\n%s", q)
	}
	if strings.Contains(q, "OFFSET") {
		t.Fatalf("entity keyset query must not contain OFFSET:\n%s", q)
	}
}

func TestEntityBatchQuery_FirstPageWithoutCursor(t *testing.T) {
	q := entityBatchQuery(false)
	if strings.Contains(q, "WHERE (created_at, id) >") {
		t.Fatalf("entity first-page query should not contain keyset WHERE:\n%s", q)
	}
	if !strings.Contains(q, "LIMIT $1") {
		t.Fatalf("entity first-page query missing limit placeholder $1:\n%s", q)
	}
	if strings.Contains(q, "OFFSET") {
		t.Fatalf("entity first-page query must not contain OFFSET:\n%s", q)
	}
}

func TestRelationBatchQuery_UsesKeysetWhenCursorPresent(t *testing.T) {
	q := relationBatchQuery(true)
	if !strings.Contains(q, "WHERE (created_at, id) > ($1, $2)") {
		t.Fatalf("relation keyset query missing cursor predicate:\n%s", q)
	}
	if !strings.Contains(q, "LIMIT $3") {
		t.Fatalf("relation keyset query missing limit placeholder $3:\n%s", q)
	}
	if strings.Contains(q, "OFFSET") {
		t.Fatalf("relation keyset query must not contain OFFSET:\n%s", q)
	}
}

func TestRelationBatchQuery_FirstPageWithoutCursor(t *testing.T) {
	q := relationBatchQuery(false)
	if strings.Contains(q, "WHERE (created_at, id) >") {
		t.Fatalf("relation first-page query should not contain keyset WHERE:\n%s", q)
	}
	if !strings.Contains(q, "LIMIT $1") {
		t.Fatalf("relation first-page query missing limit placeholder $1:\n%s", q)
	}
	if strings.Contains(q, "OFFSET") {
		t.Fatalf("relation first-page query must not contain OFFSET:\n%s", q)
	}
}
