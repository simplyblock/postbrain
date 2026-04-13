package memory

import (
	"bytes"
	"context"
	"encoding/json"
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
	"github.com/simplyblock/postbrain/internal/db/compat"
	"github.com/simplyblock/postbrain/internal/providers"
	"github.com/simplyblock/postbrain/internal/graph"
)

// embeddingService is the subset of providers.EmbeddingService used by this package.
type embeddingService interface {
	EmbedText(ctx context.Context, text string) ([]float32, error)
	EmbedCode(ctx context.Context, text string) ([]float32, error)
	TextEmbedder() embeddingIface
	CodeEmbedder() embeddingIface // may return nil
}

type embeddingResultService interface {
	EmbedTextResult(ctx context.Context, text string) (*providers.EmbedResult, error)
	EmbedCodeResult(ctx context.Context, text string) (*providers.EmbedResult, error)
}

// embeddingIface is the subset of providers.Embedder needed here.
type embeddingIface interface {
	ModelSlug() string
	Dimensions() int
}

// embeddingServiceAdapter adapts *providers.EmbeddingService to embeddingService.
type embeddingServiceAdapter struct {
	svc *providers.EmbeddingService
}

func (a *embeddingServiceAdapter) EmbedText(ctx context.Context, text string) ([]float32, error) {
	return a.svc.EmbedText(ctx, text)
}

func (a *embeddingServiceAdapter) EmbedTextResult(ctx context.Context, text string) (*providers.EmbedResult, error) {
	return a.svc.EmbedTextResult(ctx, text)
}

func (a *embeddingServiceAdapter) EmbedCode(ctx context.Context, text string) ([]float32, error) {
	return a.svc.EmbedCode(ctx, text)
}

func (a *embeddingServiceAdapter) EmbedCodeResult(ctx context.Context, text string) (*providers.EmbedResult, error) {
	return a.svc.EmbedCodeResult(ctx, text)
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
	UpdateMemoryContent(ctx context.Context, id uuid.UUID, content string, summary *string, embedding, embeddingCode []float32, textModelID, codeModelID *uuid.UUID, contentKind string, meta []byte) (*db.Memory, error)
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
	return compat.CreateMemory(ctx, p.pool, m)
}

func (p *poolMemoryDB) FindNearDuplicates(ctx context.Context, scopeID uuid.UUID, embedding []float32, threshold float64, excludeID *uuid.UUID) ([]*db.Memory, error) {
	return compat.FindNearDuplicates(ctx, p.pool, scopeID, embedding, threshold, excludeID)
}

func (p *poolMemoryDB) UpdateMemoryContent(ctx context.Context, id uuid.UUID, content string, summary *string, embedding, embeddingCode []float32, textModelID, codeModelID *uuid.UUID, contentKind string, meta []byte) (*db.Memory, error) {
	return compat.UpdateMemoryContent(ctx, p.pool, id, content, summary, embedding, embeddingCode, textModelID, codeModelID, contentKind, meta)
}

func (p *poolMemoryDB) SoftDeleteMemory(ctx context.Context, id uuid.UUID) error {
	return compat.SoftDeleteMemory(ctx, p.pool, id)
}

func (p *poolMemoryDB) UpsertEntity(ctx context.Context, e *db.Entity) (*db.Entity, error) {
	return compat.UpsertEntity(ctx, p.pool, e)
}

func (p *poolMemoryDB) LinkMemoryToEntity(ctx context.Context, memoryID, entityID uuid.UUID, role string) error {
	return compat.LinkMemoryToEntity(ctx, p.pool, memoryID, entityID, role)
}

func (p *poolMemoryDB) UpsertRelation(ctx context.Context, r *db.Relation) (*db.Relation, error) {
	return compat.UpsertRelation(ctx, p.pool, r)
}

func (p *poolMemoryDB) FindEntitiesBySuffix(ctx context.Context, scopeID uuid.UUID, suffix string) ([]*db.Entity, error) {
	return compat.FindEntitiesBySuffix(ctx, p.pool, scopeID, suffix)
}

// Store provides memory CRUD and embedding operations.
type Store struct {
	pool     *pgxpool.Pool
	svc      embeddingService
	creator  memoryDB
	repo     *db.EmbeddingRepository
	recallDB recallDB   // overridable for tests
	fanOut   fanOutFunc // overridable for tests
}

// NewStore creates a new Store backed by the given pool and embedding service.
func NewStore(pool *pgxpool.Pool, svc *providers.EmbeddingService) *Store {
	return &Store{
		pool:    pool,
		svc:     &embeddingServiceAdapter{svc: svc},
		creator: &poolMemoryDB{pool: pool},
		repo:    db.NewEmbeddingRepository(pool),
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
	Summary    *string
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

func withLongStyleMeta(meta []byte) []byte {
	const fallback = `{"content_style":"long"}`
	if len(bytes.TrimSpace(meta)) == 0 {
		return []byte(fallback)
	}

	var decoded map[string]any
	if err := json.Unmarshal(meta, &decoded); err != nil {
		return []byte(fallback)
	}
	decoded["content_style"] = "long"
	encoded, err := json.Marshal(decoded)
	if err != nil {
		return []byte(fallback)
	}
	return encoded
}

// embedResult bundles the outputs of the classify-and-embed step.
type embedResult struct {
	contentKind string
	textVec     []float32
	textModelID *uuid.UUID
	codeVec     []float32
	codeModelID *uuid.UUID
}

// classifyAndEmbed classifies the content kind and generates text and (optionally)
// code embeddings.
func (s *Store) classifyAndEmbed(ctx context.Context, input CreateInput) (embedResult, error) {
	contentKind := providers.ClassifyContent(input.Content, safeDeref(input.SourceRef))

	textVec, textModelID, err := s.embedText(ctx, input.Content)
	if err != nil {
		return embedResult{}, fmt.Errorf("memory: embed text: %w", err)
	}
	if len(textVec) == 0 {
		return embedResult{}, fmt.Errorf("memory: embed text: embedding service returned empty vector (is the model available?)")
	}

	var codeVec []float32
	var codeModelID *uuid.UUID
	if contentKind == "code" && s.svc.CodeEmbedder() != nil {
		codeVec, codeModelID, err = s.embedCode(ctx, input.Content)
		if err != nil {
			return embedResult{}, fmt.Errorf("memory: embed code: %w", err)
		}
	}

	return embedResult{
		contentKind: contentKind,
		textVec:     textVec,
		textModelID: textModelID,
		codeVec:     codeVec,
		codeModelID: codeModelID,
	}, nil
}

// insertWithDedup applies TTL logic, checks for near-duplicates, and either
// updates the existing memory or inserts a new one. Returns the persisted memory
// and whether it was a duplicate update.
func (s *Store) insertWithDedup(ctx context.Context, input CreateInput, emb embedResult) (*db.Memory, bool, error) {
	var expiresAt *time.Time
	if input.MemoryType == "working" {
		ttl := 3600
		if input.ExpiresIn != nil {
			ttl = *input.ExpiresIn
		}
		t := time.Now().Add(time.Duration(ttl) * time.Second)
		expiresAt = &t
	}

	dupes, err := s.creator.FindNearDuplicates(ctx, input.ScopeID, emb.textVec, 0.05, nil)
	if err != nil {
		return nil, false, fmt.Errorf("memory: find near duplicates: %w", err)
	}
	if len(dupes) > 0 {
		existing := dupes[0]
		updated, err := s.creator.UpdateMemoryContent(ctx, existing.ID, input.Content, input.Summary, emb.textVec, emb.codeVec, emb.textModelID, emb.codeModelID, emb.contentKind, input.Meta)
		if err != nil {
			return nil, false, fmt.Errorf("memory: update duplicate: %w", err)
		}
		if err := s.dualWriteMemoryEmbeddings(ctx, updated.ID, updated.ScopeID, emb.textVec, emb.textModelID, emb.codeVec, emb.codeModelID); err != nil {
			return nil, false, fmt.Errorf("memory: dual-write duplicate: %w", err)
		}
		return updated, true, nil
	}

	textVecVal := pgvector.NewVector(emb.textVec)
	m := &db.Memory{
		MemoryType:       input.MemoryType,
		ScopeID:          input.ScopeID,
		AuthorID:         input.AuthorID,
		Content:          input.Content,
		Summary:          input.Summary,
		Embedding:        &textVecVal,
		EmbeddingModelID: emb.textModelID,
		ContentKind:      emb.contentKind,
		Meta:             input.Meta,
		Importance:       input.Importance,
		ExpiresAt:        expiresAt,
		SourceRef:        input.SourceRef,
	}
	if len(emb.codeVec) > 0 {
		v := pgvector.NewVector(emb.codeVec)
		m.EmbeddingCode = &v
		m.EmbeddingCodeModelID = emb.codeModelID
	}
	created, err := s.creator.CreateMemory(ctx, m)
	if err != nil {
		return nil, false, fmt.Errorf("memory: create: %w", err)
	}
	if err := s.dualWriteMemoryEmbeddings(ctx, created.ID, created.ScopeID, emb.textVec, emb.textModelID, emb.codeVec, emb.codeModelID); err != nil {
		return nil, false, fmt.Errorf("memory: dual-write create: %w", err)
	}
	return created, false, nil
}

// linkMetadata links entities, optionally creates chunk child memories, and
// extracts the code graph. For duplicate-update paths isUpdate is true and chunk
// creation is skipped (chunks already exist on the original memory).
func (s *Store) linkMetadata(ctx context.Context, m *db.Memory, input CreateInput, contentKind string, isUpdate bool) error {
	if !isUpdate && utf8.RuneCountInString(input.Content) > chunking.MinContentRunes {
		s.createChunks(ctx, m.ID, m.ScopeID, m.AuthorID, input.Content, contentKind)
	}
	if err := s.linkEntitiesForMemory(ctx, m.ID, input.ScopeID, input.Entities, input.Content, input.SourceRef); err != nil {
		return err
	}
	if contentKind == "code" && input.SourceRef != nil {
		s.extractCodeGraph(ctx, input.Content, *input.SourceRef, input.ScopeID, m.ID)
	}
	return nil
}

// Create embeds, deduplicates, and persists a memory.
func (s *Store) Create(ctx context.Context, input CreateInput) (*CreateResult, error) {
	if input.Importance == 0 {
		input.Importance = 0.5
	}
	input.Meta = withLongStyleMeta(input.Meta)

	emb, err := s.classifyAndEmbed(ctx, input)
	if err != nil {
		return nil, err
	}

	m, isUpdate, err := s.insertWithDedup(ctx, input, emb)
	if err != nil {
		return nil, err
	}

	if err := s.linkMetadata(ctx, m, input, emb.contentKind, isUpdate); err != nil {
		return nil, err
	}

	action := "created"
	if isUpdate {
		action = "updated"
	}
	return &CreateResult{MemoryID: m.ID, Action: action}, nil
}

func (s *Store) linkEntitiesForMemory(
	ctx context.Context,
	memoryID, scopeID uuid.UUID,
	explicitEntities []EntityInput,
	content string,
	sourceRef *string,
) error {
	linkedEntityIDs := make(map[uuid.UUID]struct{})

	for _, ei := range explicitEntities {
		canonical := strings.ToLower(ei.Name)
		if canonical == "" {
			continue
		}
		entityType := ei.Type
		if entityType == "" {
			entityType = "concept"
		}
		entity := &db.Entity{
			ScopeID:    scopeID,
			EntityType: entityType,
			Name:       canonical,
			Canonical:  canonical,
		}
		ent, err := s.creator.UpsertEntity(ctx, entity)
		if err != nil {
			return fmt.Errorf("memory: upsert entity %q: %w", canonical, err)
		}
		if err := s.creator.LinkMemoryToEntity(ctx, memoryID, ent.ID, "related"); err != nil {
			return fmt.Errorf("memory: link entity %q: %w", canonical, err)
		}
		s.linkSameAs(ctx, scopeID, ent.ID, entity.Canonical, entity.EntityType)
		linkedEntityIDs[ent.ID] = struct{}{}
	}

	for _, e := range graph.ExtractEntitiesFromMemory(content, sourceRef) {
		e.ScopeID = scopeID
		ent, err := s.creator.UpsertEntity(ctx, e)
		if err != nil {
			continue // best-effort; don't fail the write on extraction errors
		}
		_ = s.creator.LinkMemoryToEntity(ctx, memoryID, ent.ID, "related")
		s.linkSameAs(ctx, scopeID, ent.ID, e.Canonical, e.EntityType)
		linkedEntityIDs[ent.ID] = struct{}{}
	}

	// Create co_occurs_with relations between all entities linked to this memory.
	entityIDs := make([]uuid.UUID, 0, len(linkedEntityIDs))
	for id := range linkedEntityIDs {
		entityIDs = append(entityIDs, id)
	}
	memID := memoryID
	for i := 0; i < len(entityIDs); i++ {
		for j := i + 1; j < len(entityIDs); j++ {
			if _, err := s.creator.UpsertRelation(ctx, &db.Relation{
				ScopeID:      scopeID,
				SubjectID:    entityIDs[i],
				Predicate:    "co_occurs_with",
				ObjectID:     entityIDs[j],
				Confidence:   1.0,
				SourceMemory: &memID,
			}); err != nil {
				slog.Warn("memory: co_occurs_with upsert failed", "err", err)
			}
		}
	}
	return nil
}

// Update re-embeds and persists updated content for a memory.
func (s *Store) Update(ctx context.Context, id uuid.UUID, content string, summary *string, importance float64) (*db.Memory, error) {
	contentKind := providers.ClassifyContent(content, "")
	meta := withLongStyleMeta(nil)

	textVec, textModelID, err := s.embedText(ctx, content)
	if err != nil {
		return nil, fmt.Errorf("memory: update embed text: %w", err)
	}
	if len(textVec) == 0 {
		return nil, fmt.Errorf("memory: update embed text: embedding service returned empty vector (is the model available?)")
	}

	var codeVec []float32
	var codeModelID *uuid.UUID
	if contentKind == "code" && s.svc.CodeEmbedder() != nil {
		codeVec, codeModelID, err = s.embedCode(ctx, content)
		if err != nil {
			return nil, fmt.Errorf("memory: update embed code: %w", err)
		}
	}

	updated, err := s.creator.UpdateMemoryContent(ctx, id, content, summary, textVec, codeVec, textModelID, codeModelID, contentKind, meta)
	if err != nil {
		return nil, err
	}
	if err := s.dualWriteMemoryEmbeddings(ctx, updated.ID, updated.ScopeID, textVec, textModelID, codeVec, codeModelID); err != nil {
		return nil, fmt.Errorf("memory: dual-write update: %w", err)
	}
	return updated, nil
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
	return compat.HardDeleteMemory(ctx, s.pool, id)
}

// linkSameAs connects entID to all sibling entities in scope that share the
// same canonical string but a different entity_type, mirroring the logic in
// knowledge/store.go. Non-fatal: errors are silently ignored.
func (s *Store) linkSameAs(ctx context.Context, scopeID, entID uuid.UUID, canonical, entityType string) {
	if s.pool == nil {
		return // no pool in test mode; best-effort only
	}
	siblings, err := compat.ListEntitiesByCanonical(ctx, s.pool, scopeID, canonical, entityType)
	if err != nil {
		return
	}
	for _, sib := range siblings {
		subj, obj := entID, sib.ID
		if bytes.Compare(subj[:], obj[:]) > 0 {
			subj, obj = obj, subj
		}
		_, _ = compat.UpsertRelation(ctx, s.pool, &db.Relation{
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

func (s *Store) embedText(ctx context.Context, text string) ([]float32, *uuid.UUID, error) {
	if svc, ok := s.svc.(embeddingResultService); ok {
		res, err := svc.EmbedTextResult(ctx, text)
		if err != nil {
			return nil, nil, err
		}
		if res == nil {
			return nil, nil, fmt.Errorf("nil embed result")
		}
		if res.ModelID != uuid.Nil {
			id := res.ModelID
			return res.Embedding, &id, nil
		}
		return res.Embedding, nil, nil
	}
	vec, err := s.svc.EmbedText(ctx, text)
	return vec, nil, err
}

func (s *Store) embedCode(ctx context.Context, text string) ([]float32, *uuid.UUID, error) {
	if svc, ok := s.svc.(embeddingResultService); ok {
		res, err := svc.EmbedCodeResult(ctx, text)
		if err != nil {
			return nil, nil, err
		}
		if res == nil {
			return nil, nil, fmt.Errorf("nil embed result")
		}
		if res.ModelID != uuid.Nil {
			id := res.ModelID
			return res.Embedding, &id, nil
		}
		return res.Embedding, nil, nil
	}
	vec, err := s.svc.EmbedCode(ctx, text)
	return vec, nil, err
}

func (s *Store) dualWriteMemoryEmbeddings(
	ctx context.Context,
	memoryID, scopeID uuid.UUID,
	textVec []float32,
	textModelID *uuid.UUID,
	codeVec []float32,
	codeModelID *uuid.UUID,
) error {
	if err := db.UpsertEmbeddingIfPresent(ctx, s.repo, "memory", memoryID, scopeID, textVec, textModelID); err != nil {
		return err
	}
	if err := db.UpsertEmbeddingIfPresent(ctx, s.repo, "memory", memoryID, scopeID, codeVec, codeModelID); err != nil {
		return err
	}
	return nil
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

		if _, err := s.creator.UpsertRelation(ctx, &db.Relation{
			ScopeID:      scopeID,
			SubjectID:    subjID,
			Predicate:    edge.Predicate,
			ObjectID:     objID,
			Confidence:   1.0,
			SourceMemory: &memoryID,
		}); err != nil {
			slog.Warn("memory: code-graph edge upsert failed", "err", err)
		}
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
