package skills

import (
	"context"
	"math"
	"sort"

	"github.com/google/uuid"

	"github.com/simplyblock/postbrain/internal/db"
	"github.com/simplyblock/postbrain/internal/embedding"
)

// RecallInput specifies a skill recall query.
type RecallInput struct {
	Query     string
	ScopeIDs  []uuid.UUID
	AgentType string
	Limit     int
	MinScore  float64
	Installed *bool // nil=all, true=installed only, false=not installed
	Workdir   string
}

// SkillResult is one entry in a Recall response.
type SkillResult struct {
	Skill     *db.Skill
	Score     float64
	Layer     string // always "skill"
	Installed bool
}

// computeSkillScore applies the hybrid scoring formula for skills.
// vec=0.50, bm25=0.20, importance=0.20, recency=0.10
// Skills have no decay; recency weight is fixed at 1.0.
func computeSkillScore(vecScore, bm25Score, importance, recency float64) float64 {
	return 0.50*vecScore + 0.20*bm25Score + 0.20*importance + 0.10*recency
}

// importanceFromInvocations normalizes invocation count: 100+ invocations = max (1.0).
func importanceFromInvocations(count int) float64 {
	return math.Min(1.0, float64(count)/100.0)
}

// Recall performs hybrid vector + FTS recall of published skills.
func (s *Store) Recall(ctx context.Context, svc *embedding.EmbeddingService, input RecallInput) ([]*SkillResult, error) {
	if input.Limit <= 0 {
		input.Limit = 20
	}
	if input.AgentType == "" {
		input.AgentType = "any"
	}

	// Embed query text.
	queryVec, err := svc.EmbedText(ctx, input.Query)
	if err != nil {
		return nil, err
	}

	// Vector recall.
	vecResults, err := db.RecallSkillsByVector(ctx, s.pool, input.ScopeIDs, queryVec, input.AgentType, input.Limit*2)
	if err != nil {
		return nil, err
	}

	// FTS recall.
	ftsResults, err := db.RecallSkillsByFTS(ctx, s.pool, input.ScopeIDs, input.Query, input.AgentType, input.Limit*2)
	if err != nil {
		return nil, err
	}

	// Merge by skill ID: track best vec and bm25 scores per skill.
	type entry struct {
		skill    *db.Skill
		vecScore float64
		bm25     float64
	}
	byID := make(map[uuid.UUID]*entry)

	for _, r := range vecResults {
		// For vector: distance (lower = better) comes back from pg_vector <=> operator.
		// Similarity = 1 - distance.
		sim := 1.0 - r.Score
		if e, ok := byID[r.Skill.ID]; ok {
			if sim > e.vecScore {
				e.vecScore = sim
			}
		} else {
			byID[r.Skill.ID] = &entry{skill: r.Skill, vecScore: sim}
		}
	}
	for _, r := range ftsResults {
		if e, ok := byID[r.Skill.ID]; ok {
			if r.Score > e.bm25 {
				e.bm25 = r.Score
			}
		} else {
			byID[r.Skill.ID] = &entry{skill: r.Skill, bm25: r.Score}
		}
	}

	var results []*SkillResult
	for _, e := range byID {
		importance := importanceFromInvocations(e.skill.InvocationCount)
		score := computeSkillScore(e.vecScore, e.bm25, importance, 1.0)
		if score < input.MinScore {
			continue
		}
		installed := false
		if input.Workdir != "" {
			installed = IsInstalled(e.skill.Slug, input.AgentType, input.Workdir)
		}

		results = append(results, &SkillResult{
			Skill:     e.skill,
			Score:     score,
			Layer:     "skill",
			Installed: installed,
		})
	}

	// Apply Installed filter.
	if input.Installed != nil {
		want := *input.Installed
		filtered := results[:0]
		for _, r := range results {
			if r.Installed == want {
				filtered = append(filtered, r)
			}
		}
		results = filtered
	}

	// Sort descending by score.
	sort.Slice(results, func(i, j int) bool {
		return results[i].Score > results[j].Score
	})
	if len(results) > input.Limit {
		results = results[:input.Limit]
	}

	return results, nil
}
