package jobs

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/google/uuid"

	"github.com/simplyblock/postbrain/internal/chunking"
	"github.com/simplyblock/postbrain/internal/db"
	"github.com/simplyblock/postbrain/internal/providers"
)

// ── fakeChunkBackfillStore ────────────────────────────────────────────────────

// fakeChunkBackfillStore implements chunkBackfillStore.
// memBatches / artBatches are returned in order; once exhausted, nil is returned.
type fakeChunkBackfillStore struct {
	memBatches [][]chunkBackfillRow
	artBatches [][]chunkBackfillRow
	memIdx     int
	artIdx     int
	created    []*db.Memory
	createErr  error
}

func (f *fakeChunkBackfillStore) fetchMemoriesWithoutChunks(_ context.Context, _, _ int) ([]chunkBackfillRow, error) {
	if f.memIdx >= len(f.memBatches) {
		return nil, nil
	}
	batch := f.memBatches[f.memIdx]
	f.memIdx++
	return batch, nil
}

func (f *fakeChunkBackfillStore) fetchArtifactsWithoutChunks(_ context.Context, _, _ int) ([]chunkBackfillRow, error) {
	if f.artIdx >= len(f.artBatches) {
		return nil, nil
	}
	batch := f.artBatches[f.artIdx]
	f.artIdx++
	return batch, nil
}

func (f *fakeChunkBackfillStore) createMemory(_ context.Context, m *db.Memory) (*db.Memory, error) {
	if f.createErr != nil {
		return nil, f.createErr
	}
	m.ID = uuid.New()
	f.created = append(f.created, m)
	return m, nil
}

// ── fakeTextEmbedder ─────────────────────────────────────────────────────────

// fakeTextEmbedder wraps providers.FakeEmbedder to satisfy the local textEmbedder interface.
type fakeTextEmbedder struct {
	inner *providers.FakeEmbedder
}

func newFakeTextEmbedder() *fakeTextEmbedder {
	return &fakeTextEmbedder{inner: providers.NewFakeEmbedder(4)}
}

func (f *fakeTextEmbedder) EmbedText(ctx context.Context, text string) ([]float32, error) {
	return f.inner.Embed(ctx, text)
}

// failOnceEmbedder fails EmbedText on the call indexed by failOn (0-based);
// all other calls succeed with a constant vector.
type failOnceEmbedder struct {
	failOn    int
	callCount int
}

func (f *failOnceEmbedder) EmbedText(_ context.Context, _ string) ([]float32, error) {
	n := f.callCount
	f.callCount++
	if n == f.failOn {
		return nil, errors.New("embed failed")
	}
	return []float32{0.1, 0.2, 0.3, 0.4}, nil
}

// ── helpers ───────────────────────────────────────────────────────────────────

// largeContent returns text whose rune count exceeds MinContentRunes and that
// splits into multiple chunks under DefaultChunkRunes / DefaultOverlap.
func largeContent() string {
	// ~4000 runes: well above MinContentRunes (2000) and DefaultChunkRunes (1000).
	sentence := strings.Repeat("word ", 200) + ". " // ~1002 runes
	return strings.Repeat(sentence, 4)
}

func newJob(store chunkBackfillStore, emb textEmbedder) *ChunkBackfillJob {
	return &ChunkBackfillJob{
		store:     store,
		embedder:  emb,
		batchSize: defaultChunkBackfillBatchSize,
	}
}

// ── RunMemories tests ─────────────────────────────────────────────────────────

func TestRunMemories_ZeroRows_NoEmbedderCalls(t *testing.T) {
	t.Parallel()
	store := &fakeChunkBackfillStore{} // no batches → immediately returns nil
	emb := newFakeTextEmbedder()
	j := newJob(store, emb)

	if err := j.RunMemories(context.Background()); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(store.created) != 0 {
		t.Errorf("createMemory called %d times, want 0", len(store.created))
	}
}

func TestRunMemories_NilEmbedder_IsNoop(t *testing.T) {
	t.Parallel()
	row := chunkBackfillRow{
		ID:      uuid.New(),
		ScopeID: uuid.New(),
		Content: largeContent(),
	}
	store := &fakeChunkBackfillStore{
		memBatches: [][]chunkBackfillRow{{row}},
	}
	j := newJob(store, nil) // nil embedder

	if err := j.RunMemories(context.Background()); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(store.created) != 0 {
		t.Errorf("createMemory called %d times, want 0 (nil embedder guard)", len(store.created))
	}
}

func TestRunMemories_OneLargeMemory_CreatesExpectedChunks(t *testing.T) {
	t.Parallel()
	content := largeContent()
	// Compute expected chunk count using the same parameters as the job.
	expectedChunks := len(chunking.Chunk(content, chunking.DefaultChunkRunes, chunking.DefaultOverlap))
	if expectedChunks < 2 {
		t.Fatalf("largeContent() produced only %d chunk(s) — test setup is wrong", expectedChunks)
	}

	row := chunkBackfillRow{
		ID:      uuid.New(),
		ScopeID: uuid.New(),
		Content: content,
	}
	store := &fakeChunkBackfillStore{
		memBatches: [][]chunkBackfillRow{{row}},
	}
	j := newJob(store, newFakeTextEmbedder())

	if err := j.RunMemories(context.Background()); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(store.created) != expectedChunks {
		t.Errorf("createMemory called %d times, want %d", len(store.created), expectedChunks)
	}
}

func TestRunMemories_EmbedErrorSkipsChunk_OthersStillCreated(t *testing.T) {
	t.Parallel()
	content := largeContent()
	allChunks := chunking.Chunk(content, chunking.DefaultChunkRunes, chunking.DefaultOverlap)
	if len(allChunks) < 2 {
		t.Fatal("need at least 2 chunks for this test")
	}

	row := chunkBackfillRow{
		ID:      uuid.New(),
		ScopeID: uuid.New(),
		Content: content,
	}
	store := &fakeChunkBackfillStore{
		memBatches: [][]chunkBackfillRow{{row}},
	}
	// Fail on the first embed call; all remaining chunks should still be stored.
	j := newJob(store, &failOnceEmbedder{failOn: 0})

	if err := j.RunMemories(context.Background()); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	wantCreated := len(allChunks) - 1 // one skipped due to embed failure
	if len(store.created) != wantCreated {
		t.Errorf("createMemory called %d times, want %d (one chunk skipped)", len(store.created), wantCreated)
	}
}

// ── RunArtifacts tests ────────────────────────────────────────────────────────

func TestRunArtifacts_SourceRefFormat(t *testing.T) {
	t.Parallel()
	artifactID := uuid.New()
	content := largeContent()
	expectedChunks := len(chunking.Chunk(content, chunking.DefaultChunkRunes, chunking.DefaultOverlap))

	store := &fakeChunkBackfillStore{
		artBatches: [][]chunkBackfillRow{{
			{ID: artifactID, ScopeID: uuid.New(), Content: content},
		}},
	}
	j := newJob(store, newFakeTextEmbedder())

	if err := j.RunArtifacts(context.Background()); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(store.created) != expectedChunks {
		t.Fatalf("expected %d chunks, got %d", expectedChunks, len(store.created))
	}
	for i, m := range store.created {
		wantRef := fmt.Sprintf("artifact:%s:chunk:%d", artifactID, i)
		if m.SourceRef == nil || *m.SourceRef != wantRef {
			got := "<nil>"
			if m.SourceRef != nil {
				got = *m.SourceRef
			}
			t.Errorf("chunk[%d] SourceRef = %q, want %q", i, got, wantRef)
		}
	}
}

// ── Batch pagination test ─────────────────────────────────────────────────────

func TestRunMemories_BatchPagination(t *testing.T) {
	t.Parallel()
	batchSize := 2
	content := largeContent()

	// Two rows in the first batch (full), zero in the second → loop fetches twice.
	firstBatch := []chunkBackfillRow{
		{ID: uuid.New(), ScopeID: uuid.New(), Content: content},
		{ID: uuid.New(), ScopeID: uuid.New(), Content: content},
	}
	store := &fakeChunkBackfillStore{
		memBatches: [][]chunkBackfillRow{firstBatch, {}},
	}
	j := &ChunkBackfillJob{
		store:     store,
		embedder:  newFakeTextEmbedder(),
		batchSize: batchSize,
	}

	if err := j.RunMemories(context.Background()); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// Both calls to fetchMemoriesWithoutChunks should have been made.
	if store.memIdx != 2 {
		t.Errorf("fetchMemoriesWithoutChunks called %d times, want 2 (full batch + empty batch)", store.memIdx)
	}
	// Chunks should have been created for both rows.
	chunksPerRow := len(chunking.Chunk(content, chunking.DefaultChunkRunes, chunking.DefaultOverlap))
	wantCreated := 2 * chunksPerRow
	if len(store.created) != wantCreated {
		t.Errorf("createMemory called %d times, want %d", len(store.created), wantCreated)
	}
}

func TestRunMemories_PartialBatch_StopsWithoutExtraFetch(t *testing.T) {
	t.Parallel()
	batchSize := 5
	content := largeContent()

	// Only one row returned (< batchSize) → loop stops after first fetch.
	store := &fakeChunkBackfillStore{
		memBatches: [][]chunkBackfillRow{
			{{ID: uuid.New(), ScopeID: uuid.New(), Content: content}},
		},
	}
	j := &ChunkBackfillJob{
		store:     store,
		embedder:  newFakeTextEmbedder(),
		batchSize: batchSize,
	}

	if err := j.RunMemories(context.Background()); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if store.memIdx != 1 {
		t.Errorf("fetchMemoriesWithoutChunks called %d times, want 1 (partial batch stops loop)", store.memIdx)
	}
}
