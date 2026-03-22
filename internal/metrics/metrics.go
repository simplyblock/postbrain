// Package metrics defines the Prometheus metrics exported by Postbrain.
package metrics

import (
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

var (
	// ToolDuration tracks the duration of MCP tool calls in seconds.
	ToolDuration = promauto.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "postbrain_tool_duration_seconds",
			Help:    "Duration of MCP tool calls in seconds",
			Buckets: prometheus.DefBuckets,
		},
		[]string{"tool"},
	)

	// EmbeddingDuration tracks the duration of embedding requests in seconds.
	EmbeddingDuration = promauto.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "postbrain_embedding_duration_seconds",
			Help:    "Duration of embedding requests in seconds",
			Buckets: prometheus.DefBuckets,
		},
		[]string{"backend", "model"},
	)

	// JobDuration tracks the duration of background job runs in seconds.
	JobDuration = promauto.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "postbrain_job_duration_seconds",
			Help:    "Duration of background job runs in seconds",
			Buckets: prometheus.DefBuckets,
		},
		[]string{"job"},
	)

	// ActiveMemories tracks the number of active memories per scope.
	ActiveMemories = promauto.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "postbrain_active_memories_total",
			Help: "Number of active memories per scope",
		},
		[]string{"scope"},
	)

	// RecallResults tracks the total number of recall results returned per layer.
	RecallResults = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "postbrain_recall_results_total",
			Help: "Total number of recall results returned per layer",
		},
		[]string{"layer"},
	)
)
