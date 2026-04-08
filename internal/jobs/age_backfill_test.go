package jobs

import "testing"

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
