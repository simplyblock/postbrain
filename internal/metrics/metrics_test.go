package metrics

import "testing"

// TestMetrics_CanObserveWithoutPanic verifies each metric can be used without panicking.
func TestMetrics_CanObserveWithoutPanic(t *testing.T) {
	t.Run("ToolDuration", func(t *testing.T) {
		ToolDuration.WithLabelValues("remember").Observe(0.001)
	})

	t.Run("EmbeddingDuration", func(t *testing.T) {
		EmbeddingDuration.WithLabelValues("ollama", "nomic-embed-text").Observe(0.05)
	})

	t.Run("JobDuration", func(t *testing.T) {
		JobDuration.WithLabelValues("consolidation").Observe(1.23)
	})

	t.Run("ActiveMemories", func(t *testing.T) {
		ActiveMemories.WithLabelValues("project:test").Set(42)
		ActiveMemories.WithLabelValues("project:test").Inc()
		ActiveMemories.WithLabelValues("project:test").Dec()
	})

	t.Run("RecallResults", func(t *testing.T) {
		RecallResults.WithLabelValues("memory").Add(5)
		RecallResults.WithLabelValues("knowledge").Inc()
		RecallResults.WithLabelValues("skill").Add(0)
	})
}
