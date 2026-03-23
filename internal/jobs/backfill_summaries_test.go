package jobs

import (
	"context"
	"strings"
	"testing"

	"github.com/google/uuid"
)

// fakeBackfillStore implements backfillSummaryStore for unit tests.
type fakeBackfillStore struct {
	rows    []backfillRow
	updated map[uuid.UUID]string
}

func (f *fakeBackfillStore) fetchUnsummarised(_ context.Context, batchSize, offset int) ([]backfillRow, error) {
	start := offset
	if start >= len(f.rows) {
		return nil, nil
	}
	end := start + batchSize
	if end > len(f.rows) {
		end = len(f.rows)
	}
	return f.rows[start:end], nil
}

func (f *fakeBackfillStore) setSummary(_ context.Context, id uuid.UUID, summary string) error {
	if f.updated == nil {
		f.updated = make(map[uuid.UUID]string)
	}
	f.updated[id] = summary
	return nil
}

func TestBackfillSummaries_FillsMissing(t *testing.T) {
	t.Parallel()
	longContent := strings.Repeat("word ", 200) + "end."
	id1 := uuid.New()
	id2 := uuid.New()
	store := &fakeBackfillStore{
		rows: []backfillRow{
			{ID: id1, Content: longContent},
			{ID: id2, Content: longContent},
		},
	}
	job := &BackfillSummariesJob{store: store, batchSize: 10}
	if err := job.Run(context.Background()); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(store.updated) != 2 {
		t.Errorf("expected 2 summaries written, got %d", len(store.updated))
	}
	for id, s := range store.updated {
		if s == "" {
			t.Errorf("artifact %s got empty summary", id)
		}
		if s == longContent {
			t.Errorf("artifact %s: summary equals full content (not truncated)", id)
		}
	}
}

func TestBackfillSummaries_SkipsShortContent(t *testing.T) {
	t.Parallel()
	id := uuid.New()
	store := &fakeBackfillStore{
		rows: []backfillRow{
			{ID: id, Content: "Short text."},
		},
	}
	job := &BackfillSummariesJob{store: store, batchSize: 10}
	if err := job.Run(context.Background()); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// Short content produces a summary equal to itself — we should still write
	// it so the row is no longer NULL and won't be re-visited.
	if _, ok := store.updated[id]; !ok {
		t.Error("expected summary to be written even for short content")
	}
}

func TestBackfillSummaries_EmptyTable(t *testing.T) {
	t.Parallel()
	store := &fakeBackfillStore{}
	job := &BackfillSummariesJob{store: store, batchSize: 10}
	if err := job.Run(context.Background()); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(store.updated) != 0 {
		t.Errorf("expected no updates, got %d", len(store.updated))
	}
}

func TestBackfillSummaries_PaginatesAcrossBatches(t *testing.T) {
	t.Parallel()
	var rows []backfillRow
	for i := 0; i < 25; i++ {
		rows = append(rows, backfillRow{ID: uuid.New(), Content: strings.Repeat("word ", 200) + "end."})
	}
	store := &fakeBackfillStore{rows: rows}
	job := &BackfillSummariesJob{store: store, batchSize: 10}
	if err := job.Run(context.Background()); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(store.updated) != 25 {
		t.Errorf("expected 25 summaries, got %d", len(store.updated))
	}
}
