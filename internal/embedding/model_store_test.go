package embedding

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

type fakeRow struct {
	vals []any
	err  error
}

func (r fakeRow) Scan(dest ...any) error {
	if r.err != nil {
		return r.err
	}
	for i := range dest {
		switch d := dest[i].(type) {
		case *string:
			*d = r.vals[i].(string)
		case **string:
			if r.vals[i] == nil {
				*d = nil
			} else {
				v := r.vals[i].(string)
				*d = &v
			}
		case *int:
			*d = r.vals[i].(int)
		case *uuid.UUID:
			*d = r.vals[i].(uuid.UUID)
		default:
			return errors.New("unsupported scan target")
		}
	}
	return nil
}

type fakeQueryer struct {
	rows []fakeRow
}

func (q *fakeQueryer) QueryRow(ctx context.Context, sql string, args ...any) pgx.Row {
	_ = ctx
	_ = sql
	_ = args
	if len(q.rows) == 0 {
		return fakeRow{err: pgx.ErrNoRows}
	}
	row := q.rows[0]
	q.rows = q.rows[1:]
	return row
}

func TestDBModelStore_GetModelConfig(t *testing.T) {
	t.Parallel()

	modelID := uuid.New()
	store := NewDBModelStore(&fakeQueryer{rows: []fakeRow{{vals: []any{"openai", "http://localhost:8080/v1", "text-embedding-3-small", 1536}}}})

	cfg, err := store.GetModelConfig(context.Background(), modelID)
	if err != nil {
		t.Fatalf("GetModelConfig: %v", err)
	}
	if cfg == nil {
		t.Fatal("GetModelConfig returned nil")
	}
	if cfg.ID != modelID || cfg.Provider != "openai" || cfg.ProviderModel != "text-embedding-3-small" {
		t.Fatalf("unexpected config: %+v", cfg)
	}
}

func TestDBModelStore_ActiveModelIDByContentType_NoRows(t *testing.T) {
	t.Parallel()

	store := NewDBModelStore(&fakeQueryer{rows: []fakeRow{{err: pgx.ErrNoRows}}})
	id, err := store.ActiveModelIDByContentType(context.Background(), "text")
	if err != nil {
		t.Fatalf("ActiveModelIDByContentType: %v", err)
	}
	if id != nil {
		t.Fatalf("id = %v, want nil", id)
	}
}

func TestDBModelStore_ActiveModelIDByContentType_Success(t *testing.T) {
	t.Parallel()

	modelID := uuid.New()
	store := NewDBModelStore(&fakeQueryer{rows: []fakeRow{{vals: []any{modelID}}}})
	id, err := store.ActiveModelIDByContentType(context.Background(), "text")
	if err != nil {
		t.Fatalf("ActiveModelIDByContentType: %v", err)
	}
	if id == nil || *id != modelID {
		t.Fatalf("id = %v, want %s", id, modelID)
	}
}
