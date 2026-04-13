package skills

import (
	"context"
	"math"
	"sort"
	"strings"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/simplyblock/postbrain/internal/db"
	"github.com/simplyblock/postbrain/internal/db/compat"
	"github.com/simplyblock/postbrain/internal/providers"
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
// vec=0.50, bm25=0.10, trgm=0.10, importance=0.20, recency=0.10
// Skills have no decay; recency weight is fixed at 1.0.
func computeSkillScore(vecScore, bm25Score, trgmScore, importance, recency float64) float64 {
	return 0.50*vecScore + 0.10*bm25Score + 0.10*trgmScore + 0.20*importance + 0.10*recency
}

// importanceFromInvocations normalizes invocation count: 100+ invocations = max (1.0).
func importanceFromInvocations(count int) float64 {
	return math.Min(1.0, float64(count)/100.0)
}

// Recall performs hybrid vector + FTS recall of published skills.
func (s *Store) Recall(ctx context.Context, svc *providers.EmbeddingService, input RecallInput) ([]*SkillResult, error) {
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
	vecResults := make([]db.SkillScore, 0)
	if modelID, ok := activeTextModelID(ctx, s.pool); ok {
		vecResults, err = s.recallSkillsByModelTable(ctx, modelID, input.ScopeIDs, queryVec, input.AgentType, input.Limit*2)
		if err != nil {
			return nil, err
		}
	}
	if len(vecResults) == 0 {
		vecResults, err = compat.RecallSkillsByVector(ctx, s.pool, input.ScopeIDs, queryVec, input.AgentType, input.Limit*2)
		if err != nil {
			return nil, err
		}
	}

	// FTS recall.
	ftsResults, err := compat.RecallSkillsByFTS(ctx, s.pool, input.ScopeIDs, input.Query, input.AgentType, input.Limit*2)
	if err != nil {
		return nil, err
	}

	// Trigram recall.
	trgmResults, err := compat.RecallSkillsByTrigram(ctx, s.pool, input.ScopeIDs, input.Query, input.AgentType, input.Limit*2)
	if err != nil {
		return nil, err
	}

	// Merge by skill ID: track best vec, bm25, and trgm scores per skill.
	type entry struct {
		skill    *db.Skill
		vecScore float64
		bm25     float64
		trgm     float64
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
	for _, r := range trgmResults {
		if e, ok := byID[r.Skill.ID]; ok {
			if r.Score > e.trgm {
				e.trgm = r.Score
			}
		} else {
			byID[r.Skill.ID] = &entry{skill: r.Skill, trgm: r.Score}
		}
	}

	var results []*SkillResult
	for _, e := range byID {
		importance := importanceFromInvocations(int(e.skill.InvocationCount))
		score := computeSkillScore(e.vecScore, e.bm25, e.trgm, importance, 1.0)
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

func (s *Store) recallSkillsByModelTable(
	ctx context.Context,
	modelID uuid.UUID,
	scopeIDs []uuid.UUID,
	queryVec []float32,
	agentType string,
	limit int,
) ([]db.SkillScore, error) {
	if s.repo == nil || s.pool == nil || len(queryVec) == 0 || len(scopeIDs) == 0 {
		return nil, nil
	}
	type row struct {
		id    uuid.UUID
		score float64
	}
	byID := make(map[uuid.UUID]row)
	for _, scopeID := range scopeIDs {
		scope, err := compat.GetScopeByID(ctx, s.pool, scopeID)
		if err != nil {
			return nil, err
		}
		if scope == nil {
			continue
		}
		hits, err := s.repo.QuerySimilar(ctx, db.EmbeddingQuery{
			ModelID:    modelID,
			ObjectType: "skill",
			Embedding:  queryVec,
			Limit:      limit,
			Scope: &db.ScopeFilter{
				ScopePath: scope.Path,
			},
		})
		if err != nil {
			if isModelTableUnavailableErr(err) {
				return nil, nil
			}
			return nil, err
		}
		for _, h := range hits {
			if existing, ok := byID[h.ObjectID]; !ok || h.Score > existing.score {
				byID[h.ObjectID] = row{id: h.ObjectID, score: h.Score}
			}
		}
	}
	rows := make([]db.SkillScore, 0, len(byID))
	for _, r := range byID {
		skill, err := compat.GetSkill(ctx, s.pool, r.id)
		if err != nil {
			return nil, err
		}
		if skill == nil || skill.Status != "published" || !skillMatchesAgentType(skill.AgentTypes, agentType) {
			continue
		}
		// db.SkillScore expects distance score from SQL query; convert back.
		rows = append(rows, db.SkillScore{Skill: skill, Score: 1.0 - r.score})
	}
	return rows, nil
}

func skillMatchesAgentType(agentTypes []string, agentType string) bool {
	if agentType == "any" {
		return true
	}
	for _, t := range agentTypes {
		if t == "any" || t == agentType {
			return true
		}
	}
	return false
}

func activeTextModelID(ctx context.Context, pool *pgxpool.Pool) (uuid.UUID, bool) {
	if pool == nil {
		return uuid.Nil, false
	}
	model, err := db.New(pool).GetActiveTextModel(ctx)
	if err != nil || model == nil {
		return uuid.Nil, false
	}
	return model.ID, true
}

func isModelTableUnavailableErr(err error) bool {
	if err == nil {
		return false
	}
	msg := err.Error()
	return strings.Contains(msg, "model") && (strings.Contains(msg, "not ready") || strings.Contains(msg, "not found"))
}
