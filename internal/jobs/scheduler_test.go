package jobs

import (
	"testing"
)

func TestSafeRun_CatchesPanic(t *testing.T) {
	panicked := false
	recovered := false

	fn := safeRun("test-job", func() {
		panicked = true
		panic("deliberate test panic")
	})

	// safeRun must not propagate the panic to the caller.
	func() {
		defer func() {
			if r := recover(); r != nil {
				// If we reach here, safeRun did NOT catch the panic — test fails.
				t.Errorf("safeRun propagated panic to caller: %v", r)
			}
			recovered = true
		}()
		fn()
	}()

	if !panicked {
		t.Error("expected the inner function to run and panic")
	}
	if !recovered {
		t.Error("outer recover should have been called (even if panic was already handled)")
	}
}

func TestSafeRun_NoPanic_Runs(t *testing.T) {
	ran := false
	fn := safeRun("normal-job", func() {
		ran = true
	})
	fn()
	if !ran {
		t.Error("expected wrapped function to run when no panic occurs")
	}
}
