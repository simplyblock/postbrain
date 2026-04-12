package metrics

import (
	"testing"

	"github.com/prometheus/client_golang/prometheus/testutil"
)

func TestRecallResults_RecordsIncrement(t *testing.T) {
	before := testutil.ToFloat64(RecallResults.WithLabelValues("memory"))
	RecallResults.WithLabelValues("memory").Inc()
	after := testutil.ToFloat64(RecallResults.WithLabelValues("memory"))
	if after != before+1 {
		t.Errorf("RecallResults expected increment by 1: before=%v after=%v", before, after)
	}
}

func TestScopeAuthzDenied_RecordsIncrement(t *testing.T) {
	before := testutil.ToFloat64(ScopeAuthzDenied.WithLabelValues("rest", "POST /v1/memories"))
	ScopeAuthzDenied.WithLabelValues("rest", "POST /v1/memories").Inc()
	after := testutil.ToFloat64(ScopeAuthzDenied.WithLabelValues("rest", "POST /v1/memories"))
	if after != before+1 {
		t.Errorf("ScopeAuthzDenied expected increment by 1: before=%v after=%v", before, after)
	}
}

func TestActiveMemories_SetAndChange(t *testing.T) {
	ActiveMemories.WithLabelValues("project:test").Set(10)
	if v := testutil.ToFloat64(ActiveMemories.WithLabelValues("project:test")); v != 10 {
		t.Errorf("ActiveMemories expected 10 after Set, got %v", v)
	}
	ActiveMemories.WithLabelValues("project:test").Inc()
	if v := testutil.ToFloat64(ActiveMemories.WithLabelValues("project:test")); v != 11 {
		t.Errorf("ActiveMemories expected 11 after Inc, got %v", v)
	}
}
