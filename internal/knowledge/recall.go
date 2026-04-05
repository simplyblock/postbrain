package knowledge

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/simplyblock/postbrain/internal/db"
)

// RecallInput holds parameters for knowledge artifact recall.
type RecallInput struct {
	Query    string
	ScopeID  uuid.UUID // visibility fan-out is resolved in SQL via the scope's ltree path
	Limit    int
	MinScore float64
}

// ArtifactResult pairs a knowledge artifact with its retrieval scores.
type ArtifactResult struct {
	Artifact  *db.KnowledgeArtifact
	VecScore  float64
	BM25Score float64
	TrgmScore float64
	Score     float64 // combined, with +0.1 institutional trust boost
}

// knowledgeCombinedScore computes the combined score for a knowledge artifact.
// w_vec=0.50, w_bm25=0.10, w_trgm=0.10, w_imp=0.20 (normalized endorsements), w_rec=0.10, +0.1 boost.
func knowledgeCombinedScore(vecScore, bm25Score, trgmScore, importance, recency float64) float64 {
	return 0.50*vecScore + 0.10*bm25Score + 0.10*trgmScore + 0.20*importance + 0.10*recency + 0.10
}

// normalizeEndorsements maps an endorsement count to [0, 1].
// 10+ endorsements = max importance of 1.0.
func normalizeEndorsements(count int) float64 {
	v := float64(count) / 10.0
	if v > 1.0 {
		return 1.0
	}
	return v
}

// Recall retrieves knowledge artifacts using vector similarity and FTS,
// merges the results, scores them with the institutional trust boost,
// and returns them sorted by score descending.
func (s *Store) Recall(ctx context.Context, pool *pgxpool.Pool, input RecallInput) ([]*ArtifactResult, error) {
	if input.Limit == 0 {
		input.Limit = 10
	}

	embeddingVec, _, err := s.embedContent(ctx, input.Query)
	if err != nil {
		return nil, fmt.Errorf("knowledge: recall embed: %w", err)
	}

	merged := make(map[uuid.UUID]*ArtifactResult)

	// Vector recall.
	if len(embeddingVec) > 0 {
		vecRows := make([]db.ArtifactScore, 0)
		if modelID, ok := activeTextModelID(ctx, pool); ok {
			vecRows, err = s.recallArtifactsByModelTable(ctx, pool, modelID, input.ScopeID, embeddingVec, input.Limit*2)
			if err != nil {
				return nil, fmt.Errorf("knowledge: recall by model table: %w", err)
			}
		}
		if len(vecRows) == 0 {
			vecRows, err = db.RecallArtifactsByVector(ctx, pool, input.ScopeID, embeddingVec, input.Limit*2)
			if err != nil {
				return nil, fmt.Errorf("knowledge: recall by vector: %w", err)
			}
		}
		for _, row := range vecRows {
			merged[row.Artifact.ID] = &ArtifactResult{
				Artifact: row.Artifact,
				VecScore: row.VecScore,
			}
		}
	}

	// FTS recall.
	ftsRows, err := db.RecallArtifactsByFTS(ctx, pool, input.ScopeID, input.Query, input.Limit*2)
	if err != nil {
		return nil, fmt.Errorf("knowledge: recall by fts: %w", err)
	}
	for _, row := range ftsRows {
		if existing, ok := merged[row.Artifact.ID]; ok {
			existing.BM25Score = row.BM25Score
		} else {
			merged[row.Artifact.ID] = &ArtifactResult{
				Artifact:  row.Artifact,
				BM25Score: row.BM25Score,
			}
		}
	}

	// Trigram recall.
	trgmRows, err := db.RecallArtifactsByTrigram(ctx, pool, input.ScopeID, input.Query, input.Limit*2)
	if err != nil {
		return nil, fmt.Errorf("knowledge: recall by trigram: %w", err)
	}
	for _, row := range trgmRows {
		if existing, ok := merged[row.Artifact.ID]; ok {
			existing.TrgmScore = row.TrgmScore
		} else {
			merged[row.Artifact.ID] = &ArtifactResult{
				Artifact:  row.Artifact,
				TrgmScore: row.TrgmScore,
			}
		}
	}

	// Score and filter.
	var results []*ArtifactResult
	for _, r := range merged {
		imp := normalizeEndorsements(int(r.Artifact.EndorsementCount))
		r.Score = knowledgeCombinedScore(r.VecScore, r.BM25Score, r.TrgmScore, imp, 1.0)
		if r.Score < input.MinScore {
			continue
		}
		results = append(results, r)
	}

	// Sort by score DESC.
	sort.Slice(results, func(i, j int) bool {
		return results[i].Score > results[j].Score
	})

	if len(results) > input.Limit {
		results = results[:input.Limit]
	}

	// Source suppression: remove artifacts covered by a published digest in the result set.
	results, err = suppressDigestSources(ctx, pool, results)
	if err != nil {
		// Non-fatal: log and return unsuppressed results rather than failing recall.
		_ = err
	}

	return results, nil
}

// suppressDigestSources removes source artifacts from results when a published
// digest covering them is also present in the result set.
func suppressDigestSources(ctx context.Context, pool *pgxpool.Pool, results []*ArtifactResult) ([]*ArtifactResult, error) {
	// Collect IDs of digest-type artifacts in this result set.
	var digestIDs []uuid.UUID
	for _, r := range results {
		if r.Artifact.KnowledgeType == "digest" {
			digestIDs = append(digestIDs, r.Artifact.ID)
		}
	}
	if len(digestIDs) == 0 {
		return results, nil
	}

	suppressed, err := db.GetSuppressedSourceIDs(ctx, pool, digestIDs)
	if err != nil || len(suppressed) == 0 {
		return results, err
	}

	filtered := results[:0]
	for _, r := range results {
		if _, ok := suppressed[r.Artifact.ID]; !ok {
			filtered = append(filtered, r)
		}
	}
	return filtered, nil
}

func (s *Store) recallArtifactsByModelTable(
	ctx context.Context,
	pool *pgxpool.Pool,
	modelID uuid.UUID,
	scopeID uuid.UUID,
	queryVec []float32,
	limit int,
) ([]db.ArtifactScore, error) {
	if s.repo == nil || pool == nil || len(queryVec) == 0 {
		return nil, nil
	}
	scope, err := db.GetScopeByID(ctx, pool, scopeID)
	if err != nil {
		return nil, err
	}
	if scope == nil {
		return nil, nil
	}
	hits, err := s.repo.QuerySimilar(ctx, db.EmbeddingQuery{
		ModelID:    modelID,
		ObjectType: "knowledge_artifact",
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
	rows := make([]db.ArtifactScore, 0, len(hits))
	for _, h := range hits {
		art, err := db.GetArtifact(ctx, pool, h.ObjectID)
		if err != nil {
			return nil, err
		}
		if art == nil || art.Status != "published" {
			continue
		}
		rows = append(rows, db.ArtifactScore{
			Artifact: art,
			VecScore: h.Score,
		})
	}
	return rows, nil
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
