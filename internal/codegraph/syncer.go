package codegraph

import (
	"context"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/simplyblock/postbrain/internal/db/compat"
)

// SyncState describes the current state of a repository sync for one scope.
type SyncState int

const (
	SyncIdle    SyncState = iota
	SyncRunning SyncState = iota
)

// SyncStatus holds the live and historical status for a single scope sync.
type SyncStatus struct {
	State      SyncState  `json:"state"`
	StartedAt  *time.Time `json:"started_at,omitempty"`
	FinishedAt *time.Time `json:"finished_at,omitempty"`
	CommitSHA  string     `json:"commit_sha,omitempty"`
	Error      string     `json:"error,omitempty"`
	// last successful result counters
	FilesIndexed      int `json:"files_indexed,omitempty"`
	FilesSkipped      int `json:"files_skipped,omitempty"`
	SymbolsUpserted   int `json:"symbols_upserted,omitempty"`
	RelationsUpserted int `json:"relations_upserted,omitempty"`
}

// Syncer manages background repository index runs, one per scope at a time.
type Syncer struct {
	mu     sync.Mutex
	status map[uuid.UUID]*SyncStatus
}

// NewSyncer creates a ready-to-use Syncer.
func NewSyncer() *Syncer {
	return &Syncer{status: make(map[uuid.UUID]*SyncStatus)}
}

// Status returns a copy of the current sync status for the given scope.
// Returns a zero SyncStatus (idle, no history) if nothing has ever run.
func (s *Syncer) Status(scopeID uuid.UUID) SyncStatus {
	s.mu.Lock()
	defer s.mu.Unlock()
	if st, ok := s.status[scopeID]; ok {
		return *st
	}
	return SyncStatus{}
}

// Start launches a background index run for scopeID.
// Returns false and the current status if a run is already in progress.
func (s *Syncer) Start(pool *pgxpool.Pool, opts IndexOptions) (started bool, current SyncStatus) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if st, ok := s.status[opts.ScopeID]; ok && st.State == SyncRunning {
		return false, *st
	}

	now := time.Now()
	st := &SyncStatus{State: SyncRunning, StartedAt: &now}
	s.status[opts.ScopeID] = st

	go s.run(pool, opts, st)
	return true, *st
}

func (s *Syncer) run(pool *pgxpool.Pool, opts IndexOptions, st *SyncStatus) {
	// Use a background context so the sync outlives the HTTP request.
	result, err := IndexRepo(context.Background(), pool, opts)

	now := time.Now()
	s.mu.Lock()
	defer s.mu.Unlock()

	st.State = SyncIdle
	st.FinishedAt = &now
	if err != nil {
		st.Error = err.Error()
		return
	}
	st.Error = ""
	st.CommitSHA = result.CommitSHA
	st.FilesIndexed = result.FilesIndexed
	st.FilesSkipped = result.FilesSkipped
	st.SymbolsUpserted = result.SymbolsUpserted
	st.RelationsUpserted = result.RelationsUpserted

	// Persist the newly indexed commit SHA.
	_ = compat.SetLastIndexedCommit(context.Background(), pool, opts.ScopeID, result.CommitSHA)
}
