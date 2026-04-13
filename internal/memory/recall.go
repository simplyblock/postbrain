package memory

import (
	"context"
	"log/slog"
	"math"
	"sort"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/simplyblock/postbrain/internal/db"
	"github.com/simplyblock/postbrain/internal/db/compat"
)

// RecallInput parameters for retrieving memories.
type RecallInput struct {
	Query              string
	ScopeID            uuid.UUID
	PrincipalID        uuid.UUID
	AuthorizedScopeIDs []uuid.UUID // optional intersection guard for fan-out scopes
	MemoryTypes        []string    // filter; nil = all
	SearchMode         string      // "text" | "code" | "hybrid" (default: "hybrid")
	Limit              int         // default 10
	MinScore           float64     // default 0.0
	MaxScopeDepth      int
	StrictScope        bool
	Since              *time.Time
	Until              *time.Time
}

// MemoryResult is a single recall result with its combined score.
type MemoryResult struct {
	Memory *db.Memory
	Score  float64
	Layer  string // always "memory"
}

// recallDB abstracts the DB calls needed by Recall so tests can inject mocks.
type recallDB interface {
	RecallMemoriesByVector(ctx context.Context, scopeIDs []uuid.UUID, queryVec []float32, limit int, since, until *time.Time) ([]db.MemoryScore, error)
	RecallMemoriesByFTS(ctx context.Context, scopeIDs []uuid.UUID, query string, limit int, since, until *time.Time) ([]db.MemoryScore, error)
	RecallMemoriesByTrigram(ctx context.Context, scopeIDs []uuid.UUID, query string, limit int, since, until *time.Time) ([]db.MemoryScore, error)
	RecallMemoriesByCodeVector(ctx context.Context, scopeIDs []uuid.UUID, queryVec []float32, limit int, since, until *time.Time) ([]db.MemoryScore, error)
	IncrementMemoryAccess(ctx context.Context, id uuid.UUID) error
}

// poolRecallDB wraps *pgxpool.Pool to implement recallDB.
type poolRecallDB struct {
	*poolMemoryDB
}

func (p *poolRecallDB) RecallMemoriesByVector(ctx context.Context, scopeIDs []uuid.UUID, queryVec []float32, limit int, since, until *time.Time) ([]db.MemoryScore, error) {
	return compat.RecallMemoriesByVector(ctx, p.pool, scopeIDs, queryVec, limit, since, until)
}

func (p *poolRecallDB) RecallMemoriesByFTS(ctx context.Context, scopeIDs []uuid.UUID, query string, limit int, since, until *time.Time) ([]db.MemoryScore, error) {
	return compat.RecallMemoriesByFTS(ctx, p.pool, scopeIDs, query, limit, since, until)
}

func (p *poolRecallDB) RecallMemoriesByTrigram(ctx context.Context, scopeIDs []uuid.UUID, query string, limit int, since, until *time.Time) ([]db.MemoryScore, error) {
	return compat.RecallMemoriesByTrigram(ctx, p.pool, scopeIDs, query, limit, since, until)
}

func (p *poolRecallDB) RecallMemoriesByCodeVector(ctx context.Context, scopeIDs []uuid.UUID, queryVec []float32, limit int, since, until *time.Time) ([]db.MemoryScore, error) {
	return compat.RecallMemoriesByCodeVector(ctx, p.pool, scopeIDs, queryVec, limit, since, until)
}

func (p *poolRecallDB) IncrementMemoryAccess(ctx context.Context, id uuid.UUID) error {
	return compat.IncrementMemoryAccess(ctx, p.pool, id)
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
//	score = 0.50*vec + 0.10*bm25 + 0.10*trgm + 0.20*importance + 0.10*recency
func combinedScore(vecScore, bm25Score, trgmScore, importance, recencyDecay float64) float64 {
	return 0.50*vecScore + 0.10*bm25Score + 0.10*trgmScore + 0.20*importance + 0.10*recencyDecay
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
	if len(input.AuthorizedScopeIDs) > 0 {
		scopeIDs = intersectScopeIDs(scopeIDs, input.AuthorizedScopeIDs)
		if len(scopeIDs) == 0 {
			return nil, nil
		}
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
		rows := make([]db.MemoryScore, 0)
		if codeModelID, ok := s.getActiveEmbeddingModelID(ctx, "code"); ok {
			rows, err = s.recallMemoriesByModelTable(ctx, codeModelID, codeVec, scopeIDs, input.Limit*2, input.Since, input.Until)
			if err != nil {
				return nil, err
			}
		}
		if len(rows) == 0 {
			rows, err = rdb.RecallMemoriesByCodeVector(ctx, scopeIDs, codeVec, input.Limit*2, input.Since, input.Until)
			if err != nil {
				return nil, err
			}
		}
		// Fallback: many existing code memories have no embedding_code yet.
		// If vector recall yields nothing, use lexical code search (FTS).
		if len(rows) == 0 {
			rows, err = rdb.RecallMemoriesByFTS(ctx, scopeIDs, input.Query, input.Limit*2, input.Since, input.Until)
			if err != nil {
				return nil, err
			}
		}
		for i := range rows {
			r := rows[i]
			merged[r.Memory.ID] = &r
		}
	case "text":
		rows := make([]db.MemoryScore, 0)
		if textModelID, ok := s.getActiveEmbeddingModelID(ctx, "text"); ok {
			rows, err = s.recallMemoriesByModelTable(ctx, textModelID, queryVec, scopeIDs, input.Limit*2, input.Since, input.Until)
			if err != nil {
				return nil, err
			}
		}
		if len(rows) == 0 {
			rows, err = rdb.RecallMemoriesByVector(ctx, scopeIDs, queryVec, input.Limit*2, input.Since, input.Until)
			if err != nil {
				return nil, err
			}
		}
		for i := range rows {
			r := rows[i]
			merged[r.Memory.ID] = &r
		}
	default: // "hybrid"
		vecRows := make([]db.MemoryScore, 0)
		if textModelID, ok := s.getActiveEmbeddingModelID(ctx, "text"); ok {
			vecRows, err = s.recallMemoriesByModelTable(ctx, textModelID, queryVec, scopeIDs, input.Limit*2, input.Since, input.Until)
			if err != nil {
				return nil, err
			}
		}
		if len(vecRows) == 0 {
			vecRows, err = rdb.RecallMemoriesByVector(ctx, scopeIDs, queryVec, input.Limit*2, input.Since, input.Until)
			if err != nil {
				return nil, err
			}
		}
		for i := range vecRows {
			r := vecRows[i]
			merged[r.Memory.ID] = &r
		}
		ftsRows, err := rdb.RecallMemoriesByFTS(ctx, scopeIDs, input.Query, input.Limit*2, input.Since, input.Until)
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
		trgmRows, err := rdb.RecallMemoriesByTrigram(ctx, scopeIDs, input.Query, input.Limit*2, input.Since, input.Until)
		if err != nil {
			return nil, err
		}
		for i := range trgmRows {
			r := trgmRows[i]
			if existing, ok := merged[r.Memory.ID]; ok {
				existing.TrgmScore = r.TrgmScore
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
		if !memoryWithinWindow(m, input.Since, input.Until) {
			continue
		}

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

		score := combinedScore(ms.VecScore, ms.BM25Score, ms.TrgmScore, m.Importance, recency)

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

	// 9. Async access count increment — single bounded goroutine for all results.
	if len(results) > 0 {
		ids := make([]uuid.UUID, len(results))
		for i, r := range results {
			ids[i] = r.Memory.ID
		}
		go func() {
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			for _, id := range ids {
				if err := rdb.IncrementMemoryAccess(ctx, id); err != nil {
					slog.Warn("recall: increment memory access", "id", id, "error", err)
				}
			}
		}()
	}

	return results, nil
}

func memoryWithinWindow(m *db.Memory, since, until *time.Time) bool {
	if m == nil {
		return false
	}
	ts := m.CreatedAt.UTC()
	if since != nil && ts.Before(since.UTC()) {
		return false
	}
	if until != nil && ts.After(until.UTC()) {
		return false
	}
	return true
}

func (s *Store) getActiveEmbeddingModelID(ctx context.Context, contentType string) (uuid.UUID, bool) {
	if s.pool == nil {
		return uuid.Nil, false
	}
	q := db.New(s.pool)
	switch contentType {
	case "code":
		model, err := q.GetActiveCodeModel(ctx)
		if err != nil || model == nil {
			return uuid.Nil, false
		}
		return model.ID, true
	default:
		model, err := q.GetActiveTextModel(ctx)
		if err != nil || model == nil {
			return uuid.Nil, false
		}
		return model.ID, true
	}
}

func (s *Store) recallMemoriesByModelTable(ctx context.Context, modelID uuid.UUID, queryVec []float32, scopeIDs []uuid.UUID, limit int, since, until *time.Time) ([]db.MemoryScore, error) {
	if s.repo == nil || s.pool == nil || len(queryVec) == 0 || len(scopeIDs) == 0 {
		return nil, nil
	}
	allowedScopeSet := make(map[uuid.UUID]struct{}, len(scopeIDs))
	for _, scopeID := range scopeIDs {
		allowedScopeSet[scopeID] = struct{}{}
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
		hitLimit := limit
		if since != nil || until != nil {
			hitLimit = limit * 4
		}
		hits, err := s.repo.QuerySimilar(ctx, db.EmbeddingQuery{
			ModelID:    modelID,
			ObjectType: "memory",
			Embedding:  queryVec,
			Limit:      hitLimit,
			Scope: &db.ScopeFilter{
				ScopePath: scope.Path,
			},
		})
		if err != nil {
			// During migration the active model may exist but not be ready/populated yet.
			// In that case we fallback to legacy inline-vector recall.
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
	rows := make([]db.MemoryScore, 0, len(byID))
	for _, r := range byID {
		mem, err := compat.GetMemory(ctx, s.pool, r.id)
		if err != nil {
			return nil, err
		}
		if mem == nil || !mem.IsActive {
			continue
		}
		if !memoryWithinWindow(mem, since, until) {
			continue
		}
		if _, ok := allowedScopeSet[mem.ScopeID]; !ok {
			continue
		}
		rows = append(rows, db.MemoryScore{
			Memory:   mem,
			VecScore: r.score,
		})
	}
	sort.Slice(rows, func(i, j int) bool {
		return rows[i].VecScore > rows[j].VecScore
	})
	if len(rows) > limit {
		rows = rows[:limit]
	}
	return rows, nil
}

func isModelTableUnavailableErr(err error) bool {
	if err == nil {
		return false
	}
	msg := err.Error()
	return strings.Contains(msg, "model") && (strings.Contains(msg, "not ready") || strings.Contains(msg, "not found"))
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

func intersectScopeIDs(base, allowed []uuid.UUID) []uuid.UUID {
	allowedSet := make(map[uuid.UUID]struct{}, len(allowed))
	for _, id := range allowed {
		allowedSet[id] = struct{}{}
	}
	out := make([]uuid.UUID, 0, len(base))
	for _, id := range base {
		if _, ok := allowedSet[id]; ok {
			out = append(out, id)
		}
	}
	return out
}
