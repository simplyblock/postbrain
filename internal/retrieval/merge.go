// Package retrieval provides cross-layer result merging and scoring for the
// Postbrain hybrid retrieval pipeline (memory + knowledge + skills).
package retrieval

import (
	"sort"
	"time"

	"github.com/google/uuid"
)

// Layer identifies the source layer of a retrieval result.
type Layer string

const (
	LayerMemory    Layer = "memory"
	LayerKnowledge Layer = "knowledge"
	LayerSkill     Layer = "skill"
)

// Result is a unified retrieval result from any of the three layers.
type Result struct {
	Layer Layer
	ID    uuid.UUID
	Score float64

	// Memory fields (Layer == LayerMemory)
	Content    string
	MemoryType string
	SourceRef  string
	Importance float64
	CreatedAt  time.Time

	// Knowledge fields (Layer == LayerKnowledge)
	Title         string
	KnowledgeType string
	Visibility    string
	Status        string
	Endorsements  int

	// Skill fields (Layer == LayerSkill)
	Slug            string
	Name            string
	Description     string
	AgentTypes      []string
	InvocationCount int
	Installed       bool

	// PromotedTo is set on memory results that have been promoted to a knowledge artifact.
	// Used during deduplication: if the knowledge artifact is also in the result set,
	// the memory result is dropped.
	PromotedTo *uuid.UUID
}

// RecallInput holds parameters for the multi-layer recall.
type RecallInput struct {
	Query         string
	ScopeID       uuid.UUID
	PrincipalID   uuid.UUID
	Layers        []Layer // default all three
	SearchMode    string
	AgentType     string
	Limit         int
	MinScore      float64
	MaxScopeDepth int
	StrictScope   bool
	Workdir       string
}

// CombineScores applies the standard scoring formula.
// w_vec=0.50, w_bm25=0.20, w_imp=0.20, w_rec=0.10.
// Knowledge results receive an additional +0.1 institutional trust boost.
func CombineScores(vecScore, bm25Score, importance, recencyDecay float64, layer Layer) float64 {
	score := 0.50*vecScore + 0.20*bm25Score + 0.20*importance + 0.10*recencyDecay
	if layer == LayerKnowledge {
		score += 0.10
	}
	return score
}

// Merge deduplicates and re-ranks results from multiple layers.
//
// Deduplication: if a memory result has PromotedTo set and the referenced
// knowledge artifact ID is present in the results, the memory is dropped.
func Merge(results []*Result, limit int, minScore float64) []*Result {
	if limit == 0 {
		limit = 10
	}

	// Build set of knowledge artifact IDs in the results.
	knowledgeIDs := make(map[uuid.UUID]struct{})
	for _, r := range results {
		if r.Layer == LayerKnowledge {
			knowledgeIDs[r.ID] = struct{}{}
		}
	}

	// Filter: drop promoted memories, apply minScore.
	var filtered []*Result
	for _, r := range results {
		if r.Layer == LayerMemory && r.PromotedTo != nil {
			if _, promoted := knowledgeIDs[*r.PromotedTo]; promoted {
				continue // drop: knowledge artifact supersedes this memory
			}
		}
		if r.Score < minScore {
			continue
		}
		filtered = append(filtered, r)
	}

	// Sort by score DESC.
	sort.Slice(filtered, func(i, j int) bool {
		return filtered[i].Score > filtered[j].Score
	})

	if len(filtered) > limit {
		filtered = filtered[:limit]
	}
	return filtered
}
