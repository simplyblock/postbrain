package memory

import (
	"bytes"
	"context"
	"fmt"
	"log/slog"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	pgvector "github.com/pgvector/pgvector-go"

	"github.com/simplyblock/postbrain/internal/chunking"
	"github.com/simplyblock/postbrain/internal/codegraph"
	"github.com/simplyblock/postbrain/internal/db"
	"github.com/simplyblock/postbrain/internal/embedding"
	"github.com/simplyblock/postbrain/internal/graph"
)

// embeddingService is the subset of embedding.EmbeddingService used by this package.
type embeddingService interface {
	EmbedText(ctx context.Context, text string) ([]float32, error)
	EmbedCode(ctx context.Context, text string) ([]float32, error)
	TextEmbedder() embeddingIface
	CodeEmbedder() embeddingIface // may return nil
}

// embeddingIface is the subset of embedding.Embedder needed here.
type embeddingIface interface {
	ModelSlug() string
	Dimensions() int
}

// embeddingServiceAdapter adapts *embedding.EmbeddingService to embeddingService.
type embeddingServiceAdapter struct {
	svc *embedding.EmbeddingService
}

func (a *embeddingServiceAdapter) EmbedText(ctx context.Context, text string) ([]float32, error) {
	return a.svc.EmbedText(ctx, text)
}

func (a *embeddingServiceAdapter) EmbedCode(ctx context.Context, text string) ([]float32, error) {
	return a.svc.EmbedCode(ctx, text)
}

func (a *embeddingServiceAdapter) TextEmbedder() embeddingIface {
	return a.svc.TextEmbedder()
}

func (a *embeddingServiceAdapter) CodeEmbedder() embeddingIface {
	return a.svc.CodeEmbedder()
}

// memoryDB abstracts the database operations used by Store so tests can inject
// a mock without requiring a real PostgreSQL connection.
type memoryDB interface {
	CreateMemory(ctx context.Context, m *db.Memory) (*db.Memory, error)
	FindNearDuplicates(ctx context.Context, scopeID uuid.UUID, embedding []float32, threshold float64, excludeID *uuid.UUID) ([]*db.Memory, error)
	UpdateMemoryContent(ctx context.Context, id uuid.UUID, content string, embedding, embeddingCode []float32, textModelID, codeModelID *uuid.UUID, contentKind string) (*db.Memory, error)
	SoftDeleteMemory(ctx context.Context, id uuid.UUID) error
	UpsertEntity(ctx context.Context, e *db.Entity) (*db.Entity, error)
	LinkMemoryToEntity(ctx context.Context, memoryID, entityID uuid.UUID, role string) error
	UpsertRelation(ctx context.Context, r *db.Relation) (*db.Relation, error)
	FindEntitiesBySuffix(ctx context.Context, scopeID uuid.UUID, suffix string) ([]*db.Entity, error)
}

// poolMemoryDB wraps a real pgxpool.Pool to implement memoryDB.
type poolMemoryDB struct {
	pool *pgxpool.Pool
}

func (p *poolMemoryDB) CreateMemory(ctx context.Context, m *db.Memory) (*db.Memory, error) {
	return db.CreateMemory(ctx, p.pool, m)
}

func (p *poolMemoryDB) FindNearDuplicates(ctx context.Context, scopeID uuid.UUID, embedding []float32, threshold float64, excludeID *uuid.UUID) ([]*db.Memory, error) {
	return db.FindNearDuplicates(ctx, p.pool, scopeID, embedding, threshold, excludeID)
}

func (p *poolMemoryDB) UpdateMemoryContent(ctx context.Context, id uuid.UUID, content string, embedding, embeddingCode []float32, textModelID, codeModelID *uuid.UUID, contentKind string) (*db.Memory, error) {
	return db.UpdateMemoryContent(ctx, p.pool, id, content, embedding, embeddingCode, textModelID, codeModelID, contentKind)
}

func (p *poolMemoryDB) SoftDeleteMemory(ctx context.Context, id uuid.UUID) error {
	return db.SoftDeleteMemory(ctx, p.pool, id)
}

func (p *poolMemoryDB) UpsertEntity(ctx context.Context, e *db.Entity) (*db.Entity, error) {
	return db.UpsertEntity(ctx, p.pool, e)
}

func (p *poolMemoryDB) LinkMemoryToEntity(ctx context.Context, memoryID, entityID uuid.UUID, role string) error {
	return db.LinkMemoryToEntity(ctx, p.pool, memoryID, entityID, role)
}

func (p *poolMemoryDB) UpsertRelation(ctx context.Context, r *db.Relation) (*db.Relation, error) {
	return db.UpsertRelation(ctx, p.pool, r)
}

func (p *poolMemoryDB) FindEntitiesBySuffix(ctx context.Context, scopeID uuid.UUID, suffix string) ([]*db.Entity, error) {
	return db.FindEntitiesBySuffix(ctx, p.pool, scopeID, suffix)
}

// Store provides memory CRUD and embedding operations.
type Store struct {
	pool     *pgxpool.Pool
	svc      embeddingService
	creator  memoryDB
	recallDB recallDB   // overridable for tests
	fanOut   fanOutFunc // overridable for tests
}

// NewStore creates a new Store backed by the given pool and embedding service.
func NewStore(pool *pgxpool.Pool, svc *embedding.EmbeddingService) *Store {
	return &Store{
		pool:    pool,
		svc:     &embeddingServiceAdapter{svc: svc},
		creator: &poolMemoryDB{pool: pool},
	}
}

// EntityInput carries an explicitly named entity with its type for use in CreateInput.
type EntityInput struct {
	Name string // canonical name, e.g. "postgresql", "src/auth.go"
	Type string // entity_type: concept|technology|file|person|service|pr|decision|…
}

// CreateInput holds the fields required to create a new memory.
type CreateInput struct {
	Content    string
	MemoryType string // semantic|episodic|procedural|working
	ScopeID    uuid.UUID
	AuthorID   uuid.UUID
	Importance float64 // 0–1, default 0.5
	SourceRef  *string
	Entities   []EntityInput // explicit entities to link
	ExpiresIn  *int          // seconds; meaningful only for working memory
	Meta       []byte
}

// CreateResult is returned by Create.
type CreateResult struct {
	MemoryID uuid.UUID
	Action   string // "created" | "updated"
}

// Create embeds, deduplicates, and persists a memory.
func (s *Store) Create(ctx context.Context, input CreateInput) (*CreateResult, error) {
	if input.Importance == 0 {
		input.Importance = 0.5
	}

	// 1. Classify content kind.
	contentKind := embedding.ClassifyContent(input.Content, safeDeref(input.SourceRef))

	// 2. Embed text.
	textVec, err := s.svc.EmbedText(ctx, input.Content)
	if err != nil {
		return nil, fmt.Errorf("memory: embed text: %w", err)
	}
	if len(textVec) == 0 {
		return nil, fmt.Errorf("memory: embed text: embedding service returned empty vector (is the model available?)")
	}

	// 3. Embed code if content_kind == "code" and a code embedder is available.
	var codeVec []float32
	if contentKind == "code" && s.svc.CodeEmbedder() != nil {
		codeVec, err = s.svc.EmbedCode(ctx, input.Content)
		if err != nil {
			return nil, fmt.Errorf("memory: embed code: %w", err)
		}
	}

	// 4. TTL logic.
	var expiresAt *time.Time
	if input.MemoryType == "working" {
		ttl := 3600
		if input.ExpiresIn != nil {
			ttl = *input.ExpiresIn
		}
		t := time.Now().Add(time.Duration(ttl) * time.Second)
		expiresAt = &t
	}

	// 5. Near-duplicate check.
	dupes, err := s.creator.FindNearDuplicates(ctx, input.ScopeID, textVec, 0.05, nil)
	if err != nil {
		return nil, fmt.Errorf("memory: find near duplicates: %w", err)
	}
	if len(dupes) > 0 {
		existing := dupes[0]
		updated, err := s.creator.UpdateMemoryContent(ctx, existing.ID, input.Content, textVec, codeVec, nil, nil, contentKind)
		if err != nil {
			return nil, fmt.Errorf("memory: update duplicate: %w", err)
		}
		return &CreateResult{MemoryID: updated.ID, Action: "updated"}, nil
	}

	// 6. Insert.
	textVecVal := pgvector.NewVector(textVec)
	m := &db.Memory{
		MemoryType:  input.MemoryType,
		ScopeID:     input.ScopeID,
		AuthorID:    input.AuthorID,
		Content:     input.Content,
		Embedding:   &textVecVal,
		ContentKind: contentKind,
		Meta:        input.Meta,
		Importance:  input.Importance,
		ExpiresAt:   expiresAt,
		SourceRef:   input.SourceRef,
	}
	if len(codeVec) > 0 {
		v := pgvector.NewVector(codeVec)
		m.EmbeddingCode = &v
	}
	created, err := s.creator.CreateMemory(ctx, m)
	if err != nil {
		return nil, fmt.Errorf("memory: create: %w", err)
	}

	// 7. Create chunk child memories for large content so recall can surface
	// specific passages rather than the whole document's averaged embedding.
	if utf8.RuneCountInString(input.Content) > chunking.MinContentRunes {
		s.createChunks(ctx, created.ID, created.ScopeID, created.AuthorID, input.Content, contentKind)
	}

	// 8. Link explicit entities and auto-extracted entities.
	linkedEntityIDs := make(map[uuid.UUID]struct{})

	for _, ei := range input.Entities {
		canonical := strings.ToLower(ei.Name)
		if canonical == "" {
			continue
		}
		entityType := ei.Type
		if entityType == "" {
			entityType = "concept"
		}
		entity := &db.Entity{
			ScopeID:    input.ScopeID,
			EntityType: entityType,
			Name:       canonical,
			Canonical:  canonical,
		}
		ent, err := s.creator.UpsertEntity(ctx, entity)
		if err != nil {
			return nil, fmt.Errorf("memory: upsert entity %q: %w", canonical, err)
		}
		if err := s.creator.LinkMemoryToEntity(ctx, created.ID, ent.ID, "related"); err != nil {
			return nil, fmt.Errorf("memory: link entity %q: %w", canonical, err)
		}
		s.linkSameAs(ctx, input.ScopeID, ent.ID, entity.Canonical, entity.EntityType)
		linkedEntityIDs[ent.ID] = struct{}{}
	}

	for _, e := range graph.ExtractEntitiesFromMemory(input.Content, input.SourceRef) {
		e.ScopeID = input.ScopeID
		ent, err := s.creator.UpsertEntity(ctx, e)
		if err != nil {
			continue // best-effort; don't fail the write on extraction errors
		}
		_ = s.creator.LinkMemoryToEntity(ctx, created.ID, ent.ID, "related")
		s.linkSameAs(ctx, input.ScopeID, ent.ID, e.Canonical, e.EntityType)
		linkedEntityIDs[ent.ID] = struct{}{}
	}

	// 8. Create co_occurs_with relations between all entities linked to this memory.
	entityIDs := make([]uuid.UUID, 0, len(linkedEntityIDs))
	for id := range linkedEntityIDs {
		entityIDs = append(entityIDs, id)
	}
	memID := created.ID
	for i := 0; i < len(entityIDs); i++ {
		for j := i + 1; j < len(entityIDs); j++ {
			_, _ = s.creator.UpsertRelation(ctx, &db.Relation{
				ScopeID:      input.ScopeID,
				SubjectID:    entityIDs[i],
				Predicate:    "co_occurs_with",
				ObjectID:     entityIDs[j],
				Confidence:   1.0,
				SourceMemory: &memID,
			})
		}
	}

	// 9. Code graph extraction for code memories with a file: source_ref.
	if contentKind == "code" && input.SourceRef != nil {
		s.extractCodeGraph(ctx, input.Content, *input.SourceRef, input.ScopeID, created.ID)
	}

	return &CreateResult{MemoryID: created.ID, Action: "created"}, nil
}

// Update re-embeds and persists updated content for a memory.
func (s *Store) Update(ctx context.Context, id uuid.UUID, content string, importance float64) (*db.Memory, error) {
	contentKind := embedding.ClassifyContent(content, "")

	textVec, err := s.svc.EmbedText(ctx, content)
	if err != nil {
		return nil, fmt.Errorf("memory: update embed text: %w", err)
	}
	if len(textVec) == 0 {
		return nil, fmt.Errorf("memory: update embed text: embedding service returned empty vector (is the model available?)")
	}

	var codeVec []float32
	if contentKind == "code" && s.svc.CodeEmbedder() != nil {
		codeVec, err = s.svc.EmbedCode(ctx, content)
		if err != nil {
			return nil, fmt.Errorf("memory: update embed code: %w", err)
		}
	}

	return s.creator.UpdateMemoryContent(ctx, id, content, textVec, codeVec, nil, nil, contentKind)
}

// SoftDelete marks a memory as inactive.
func (s *Store) SoftDelete(ctx context.Context, id uuid.UUID) error {
	return s.creator.SoftDeleteMemory(ctx, id)
}

// HardDelete permanently removes a memory.
func (s *Store) HardDelete(ctx context.Context, id uuid.UUID) error {
	if s.pool == nil {
		return fmt.Errorf("memory: hard delete: pool is nil")
	}
	return db.HardDeleteMemory(ctx, s.pool, id)
}

// linkSameAs connects entID to all sibling entities in scope that share the
// same canonical string but a different entity_type, mirroring the logic in
// knowledge/store.go. Non-fatal: errors are silently ignored.
func (s *Store) linkSameAs(ctx context.Context, scopeID, entID uuid.UUID, canonical, entityType string) {
	if s.pool == nil {
		return // no pool in test mode; best-effort only
	}
	siblings, err := db.ListEntitiesByCanonical(ctx, s.pool, scopeID, canonical, entityType)
	if err != nil {
		return
	}
	for _, sib := range siblings {
		subj, obj := entID, sib.ID
		if bytes.Compare(subj[:], obj[:]) > 0 {
			subj, obj = obj, subj
		}
		_, _ = db.UpsertRelation(ctx, s.pool, &db.Relation{
			ScopeID:    scopeID,
			SubjectID:  subj,
			Predicate:  "same_as",
			ObjectID:   obj,
			Confidence: 1.0,
		})
	}
}

// safeDeref returns "" if s is nil, else *s.
func safeDeref(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}

// extractCodeGraph runs tree-sitter extraction on a code memory and upserts
// the resulting symbols (as entities) and edges (as relations) into the graph.
// All errors are best-effort — a parse failure never blocks the memory write.
func (s *Store) extractCodeGraph(ctx context.Context, content, sourceRef string, scopeID, memoryID uuid.UUID) {
	// source_ref format: file:path/to/file.go  or  file:path/to/file.go:42
	filePath := strings.TrimPrefix(sourceRef, "file:")
	if filePath == sourceRef {
		return // not a file: ref
	}
	// Strip optional trailing :line or :line:col
	if idx := strings.LastIndex(filePath, ":"); idx > 0 {
		// Only strip if what follows looks like a number (line reference).
		tail := filePath[idx+1:]
		allDigits := len(tail) > 0
		for _, c := range tail {
			if c < '0' || c > '9' {
				allDigits = false
				break
			}
		}
		if allDigits {
			filePath = filePath[:idx]
		}
	}

	syms, edges, err := codegraph.Extract(ctx, []byte(content), filePath)
	if err != nil {
		return // unsupported language or parse error — skip silently
	}

	// Build a map of symbol name → entity ID for relation linking.
	symToID := make(map[string]uuid.UUID, len(syms))

	for _, sym := range syms {
		ent, err := s.creator.UpsertEntity(ctx, &db.Entity{
			ScopeID:    scopeID,
			EntityType: string(sym.Kind),
			Name:       sym.Name,
			Canonical:  sym.Name,
		})
		if err != nil {
			continue
		}
		symToID[sym.Name] = ent.ID
		// Link the file-level symbol (KindFile) to this memory.
		if sym.Kind == codegraph.KindFile {
			_ = s.creator.LinkMemoryToEntity(ctx, memoryID, ent.ID, "source")
		}
		// Connect to any sibling entities with the same canonical but different type.
		s.linkSameAs(ctx, scopeID, ent.ID, sym.Name, string(sym.Kind))
	}

	// Upsert edges, resolving object names heuristically when not in symToID.
	for _, edge := range edges {
		subjID, ok := symToID[edge.SubjectName]
		if !ok {
			continue // unknown subject — skip
		}

		objID, ok := symToID[edge.ObjectName]
		if !ok {
			// Heuristic: search for an entity in this scope whose canonical
			// name ends with the target name.
			candidates, err := s.creator.FindEntitiesBySuffix(ctx, scopeID, edge.ObjectName)
			if err != nil || len(candidates) == 0 {
				continue // unresolved — skip
			}
			objID = candidates[0].ID
		}

		_, _ = s.creator.UpsertRelation(ctx, &db.Relation{
			ScopeID:      scopeID,
			SubjectID:    subjID,
			Predicate:    edge.Predicate,
			ObjectID:     objID,
			Confidence:   1.0,
			SourceMemory: &memoryID,
		})
	}
}

// createChunks splits large content and stores each chunk as a child memory
// with parent_memory_id pointing back to parentID. Errors are logged, not
// returned — chunk creation is best-effort; the parent memory already exists.
func (s *Store) createChunks(ctx context.Context, parentID, scopeID, authorID uuid.UUID, content, contentKind string) {
	chunks := chunking.Chunk(content, chunking.DefaultChunkRunes, chunking.DefaultOverlap)
	if len(chunks) <= 1 {
		return
	}
	for i, chunk := range chunks {
		vec, err := s.svc.EmbedText(ctx, chunk)
		if err != nil {
			slog.WarnContext(ctx, "memory: chunk embed failed", "chunk", i, "parent_id", parentID, "err", err)
			continue
		}
		ref := fmt.Sprintf("chunk:%d", i)
		v := pgvector.NewVector(vec)
		m := &db.Memory{
			MemoryType:      "semantic",
			ScopeID:         scopeID,
			AuthorID:        authorID,
			Content:         chunk,
			ContentKind:     contentKind,
			Embedding:       &v,
			SourceRef:       &ref,
			ParentMemoryID:  &parentID,
			PromotionStatus: "none",
		}
		if _, err := s.creator.CreateMemory(ctx, m); err != nil {
			slog.WarnContext(ctx, "memory: chunk store failed", "chunk", i, "parent_id", parentID, "err", err)
		}
	}
	slog.InfoContext(ctx, "memory: created chunks", "parent_id", parentID, "count", len(chunks))
}
