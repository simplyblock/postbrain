package memory

import (
	"context"
	"math"
	"sort"
	"time"

	"github.com/google/uuid"

	"github.com/simplyblock/postbrain/internal/db"
)

// RecallInput parameters for retrieving memories.
type RecallInput struct {
	Query         string
	ScopeID       uuid.UUID
	PrincipalID   uuid.UUID
	MemoryTypes   []string // filter; nil = all
	SearchMode    string   // "text" | "code" | "hybrid" (default: "hybrid")
	Limit         int      // default 10
	MinScore      float64  // default 0.0
	MaxScopeDepth int
	StrictScope   bool
}

// MemoryResult is a single recall result with its combined score.
type MemoryResult struct {
	Memory *db.Memory
	Score  float64
	Layer  string // always "memory"
}

// recallDB abstracts the DB calls needed by Recall so tests can inject mocks.
type recallDB interface {
	RecallMemoriesByVector(ctx context.Context, scopeIDs []uuid.UUID, queryVec []float32, limit int) ([]db.MemoryScore, error)
	RecallMemoriesByFTS(ctx context.Context, scopeIDs []uuid.UUID, query string, limit int) ([]db.MemoryScore, error)
	RecallMemoriesByCodeVector(ctx context.Context, scopeIDs []uuid.UUID, queryVec []float32, limit int) ([]db.MemoryScore, error)
	IncrementMemoryAccess(ctx context.Context, id uuid.UUID) error
}

// poolRecallDB wraps *pgxpool.Pool to implement recallDB.
type poolRecallDB struct {
	*poolMemoryDB
}

func (p *poolRecallDB) RecallMemoriesByVector(ctx context.Context, scopeIDs []uuid.UUID, queryVec []float32, limit int) ([]db.MemoryScore, error) {
	return db.RecallMemoriesByVector(ctx, p.pool, scopeIDs, queryVec, limit)
}

func (p *poolRecallDB) RecallMemoriesByFTS(ctx context.Context, scopeIDs []uuid.UUID, query string, limit int) ([]db.MemoryScore, error) {
	return db.RecallMemoriesByFTS(ctx, p.pool, scopeIDs, query, limit)
}

func (p *poolRecallDB) RecallMemoriesByCodeVector(ctx context.Context, scopeIDs []uuid.UUID, queryVec []float32, limit int) ([]db.MemoryScore, error) {
	return db.RecallMemoriesByCodeVector(ctx, p.pool, scopeIDs, queryVec, limit)
}

func (p *poolRecallDB) IncrementMemoryAccess(ctx context.Context, id uuid.UUID) error {
	return db.IncrementMemoryAccess(ctx, p.pool, id)
}

// fanOutFunc is a dependency-injected fan-out function for testing.
type fanOutFunc func(ctx context.Context, scopeID, principalID uuid.UUID, maxDepth int, strictScope bool) ([]uuid.UUID, error)

// DecayLambda returns the decay constant λ for the given memory type.
func DecayLambda(memoryType string) float64 {
	switch memoryType {
	case "working":
		return 0.015
	case "episodic":
		return 0.010
	default:
		return 0.005
	}
}

// combinedScore computes the weighted combined score.
//
//	score = 0.50*vecScore + 0.20*bm25Score + 0.20*importance + 0.10*recencyDecay
func combinedScore(vecScore, bm25Score, importance, recencyDecay float64) float64 {
	return 0.50*vecScore + 0.20*bm25Score + 0.20*importance + 0.10*recencyDecay
}

// Recall retrieves memories across multiple scopes using hybrid search.
func (s *Store) Recall(ctx context.Context, input RecallInput) ([]*MemoryResult, error) {
	if input.Limit == 0 {
		input.Limit = 10
	}
	if input.SearchMode == "" {
		input.SearchMode = "hybrid"
	}

	// 1. Fan-out scope IDs.
	var scopeIDs []uuid.UUID
	var err error
	if s.fanOut != nil {
		scopeIDs, err = s.fanOut(ctx, input.ScopeID, input.PrincipalID, input.MaxScopeDepth, input.StrictScope)
	} else {
		scopeIDs, err = FanOutScopeIDs(ctx, s.pool, input.ScopeID, input.PrincipalID, input.MaxScopeDepth, input.StrictScope)
	}
	if err != nil {
		return nil, err
	}

	rdb := s.recallDB
	if rdb == nil {
		rdb = &poolRecallDB{&poolMemoryDB{pool: s.pool}}
	}

	// 2. Embed query.
	queryVec, err := s.svc.EmbedText(ctx, input.Query)
	if err != nil {
		return nil, err
	}

	// 3. Query by search mode and merge results by memory ID.
	merged := make(map[uuid.UUID]*db.MemoryScore)

	switch input.SearchMode {
	case "code":
		codeVec, err := s.svc.EmbedCode(ctx, input.Query)
		if err != nil {
			return nil, err
		}
		rows, err := rdb.RecallMemoriesByCodeVector(ctx, scopeIDs, codeVec, input.Limit*2)
		if err != nil {
			return nil, err
		}
		for i := range rows {
			r := rows[i]
			merged[r.Memory.ID] = &r
		}
	case "text":
		rows, err := rdb.RecallMemoriesByVector(ctx, scopeIDs, queryVec, input.Limit*2)
		if err != nil {
			return nil, err
		}
		for i := range rows {
			r := rows[i]
			merged[r.Memory.ID] = &r
		}
	default: // "hybrid"
		vecRows, err := rdb.RecallMemoriesByVector(ctx, scopeIDs, queryVec, input.Limit*2)
		if err != nil {
			return nil, err
		}
		for i := range vecRows {
			r := vecRows[i]
			merged[r.Memory.ID] = &r
		}
		ftsRows, err := rdb.RecallMemoriesByFTS(ctx, scopeIDs, input.Query, input.Limit*2)
		if err != nil {
			return nil, err
		}
		for i := range ftsRows {
			r := ftsRows[i]
			if existing, ok := merged[r.Memory.ID]; ok {
				existing.BM25Score = r.BM25Score
			} else {
				merged[r.Memory.ID] = &r
			}
		}
	}

	// 4. Compute combined scores and build results.
	now := time.Now()
	var results []*MemoryResult
	for _, ms := range merged {
		m := ms.Memory

		// 5. Apply MemoryTypes filter.
		if len(input.MemoryTypes) > 0 && !containsString(input.MemoryTypes, m.MemoryType) {
			continue
		}

		var ref time.Time
		if m.LastAccessed != nil {
			ref = *m.LastAccessed
		} else {
			ref = m.CreatedAt
		}
		days := now.Sub(ref).Hours() / 24
		λ := DecayLambda(m.MemoryType)
		recency := math.Exp(-λ * days)

		score := combinedScore(ms.VecScore, ms.BM25Score, m.Importance, recency)

		// 6. Filter by MinScore.
		if score < input.MinScore {
			continue
		}

		results = append(results, &MemoryResult{
			Memory: m,
			Score:  score,
			Layer:  "memory",
		})
	}

	// 7. Sort by score DESC.
	sort.Slice(results, func(i, j int) bool {
		return results[i].Score > results[j].Score
	})

	// 8. Truncate to limit.
	if len(results) > input.Limit {
		results = results[:input.Limit]
	}

	// 9. Async access count increment.
	for _, r := range results {
		id := r.Memory.ID
		go func() {
			_ = rdb.IncrementMemoryAccess(context.Background(), id)
		}()
	}

	return results, nil
}

// containsString reports whether s is in the slice.
func containsString(slice []string, s string) bool {
	for _, v := range slice {
		if v == s {
			return true
		}
	}
	return false
}
