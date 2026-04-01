package codegraph

import (
	"net"
	"testing"
	"time"

	"github.com/google/uuid"
)

// ── NewSyncer ─────────────────────────────────────────────────────────────────

func TestNewSyncer_NotNil(t *testing.T) {
	t.Parallel()
	if NewSyncer() == nil {
		t.Fatal("NewSyncer returned nil")
	}
}

// ── Status ────────────────────────────────────────────────────────────────────

func TestSyncer_Status_IdleForUnknownScope(t *testing.T) {
	t.Parallel()
	s := NewSyncer()
	st := s.Status(uuid.New())
	if st.State != SyncIdle {
		t.Errorf("State = %v, want SyncIdle", st.State)
	}
	if st.StartedAt != nil {
		t.Errorf("StartedAt = %v, want nil", st.StartedAt)
	}
	if st.Error != "" {
		t.Errorf("Error = %q, want empty", st.Error)
	}
}

func TestSyncer_Status_ReturnsCopy(t *testing.T) {
	t.Parallel()
	s := NewSyncer()
	scopeID := uuid.New()
	now := time.Now()

	// Inject a known state directly (same package).
	s.status[scopeID] = &SyncStatus{
		State:     SyncRunning,
		StartedAt: &now,
		Error:     "original error",
	}

	// Mutate the first copy.
	copy1 := s.Status(scopeID)
	copy1.Error = "mutated"
	copy1.State = SyncIdle
	copy1.StartedAt = nil

	// A second call must reflect the original, unmutated internal state.
	copy2 := s.Status(scopeID)
	if copy2.State != SyncRunning {
		t.Errorf("State = %v after copy mutation, want SyncRunning", copy2.State)
	}
	if copy2.Error != "original error" {
		t.Errorf("Error = %q after copy mutation, want %q", copy2.Error, "original error")
	}
	if copy2.StartedAt == nil {
		t.Error("StartedAt = nil after copy mutation, want non-nil")
	}
}

// ── Start ─────────────────────────────────────────────────────────────────────

// hangingListener opens a TCP listener that accepts connections but never
// responds, keeping any git clone attempt alive until the listener is closed.
// This lets us verify the SyncRunning state before IndexRepo returns.
func hangingListener(t *testing.T) *net.TCPListener {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("hangingListener: %v", err)
	}
	// Accept in background so the dialer doesn't get ECONNREFUSED.
	go func() {
		for {
			conn, err := ln.Accept()
			if err != nil {
				return // listener closed
			}
			_ = conn.Close() // accept then immediately close — causes a quick error
		}
	}()
	return ln.(*net.TCPListener)
}

func TestSyncer_Start_ReturnsTrueForNewScope(t *testing.T) {
	t.Parallel()
	ln := hangingListener(t)
	defer func() {
		_ = ln.Close()
	}()

	s := NewSyncer()
	opts := IndexOptions{
		ScopeID: uuid.New(),
		RepoURL: "http://" + ln.Addr().String() + "/repo.git",
	}

	started, status := s.Start(nil, opts)

	if !started {
		t.Error("expected started=true for a new scope")
	}
	if status.State != SyncRunning {
		t.Errorf("State = %v, want SyncRunning", status.State)
	}
	if status.StartedAt == nil {
		t.Error("StartedAt is nil, want a timestamp")
	}
}

func TestSyncer_Start_ReturnsFalseWhenAlreadyRunning(t *testing.T) {
	t.Parallel()
	s := NewSyncer()
	scopeID := uuid.New()
	now := time.Now()

	// Pre-populate internal state as running (same package access).
	s.status[scopeID] = &SyncStatus{State: SyncRunning, StartedAt: &now}

	started, current := s.Start(nil, IndexOptions{ScopeID: scopeID})

	if started {
		t.Error("expected started=false when a run is already in progress")
	}
	if current.State != SyncRunning {
		t.Errorf("returned State = %v, want SyncRunning", current.State)
	}
}

func TestSyncer_Start_SecondScopeStartsIndependently(t *testing.T) {
	t.Parallel()
	s := NewSyncer()
	scopeA := uuid.New()
	scopeB := uuid.New()
	now := time.Now()

	// Scope A is already running.
	s.status[scopeA] = &SyncStatus{State: SyncRunning, StartedAt: &now}

	ln := hangingListener(t)
	defer func() {
		_ = ln.Close()
	}()

	// Scope B should start regardless.
	started, status := s.Start(nil, IndexOptions{
		ScopeID: scopeB,
		RepoURL: "http://" + ln.Addr().String() + "/repo.git",
	})

	if !started {
		t.Error("scope B should start independently of scope A")
	}
	if status.State != SyncRunning {
		t.Errorf("scope B State = %v, want SyncRunning", status.State)
	}
}
