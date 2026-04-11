package embedding

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/simplyblock/postbrain/internal/config"
)

// EnableModelDrivenFactory configures model-aware embedder resolution from DB.
// It loads currently active text/code model IDs and binds a DB-backed factory.
func (s *EmbeddingService) EnableModelDrivenFactory(ctx context.Context, pool *pgxpool.Pool, cfg *config.EmbeddingConfig) error {
	if s == nil {
		return fmt.Errorf("embedding service is nil")
	}
	if pool == nil {
		return fmt.Errorf("embedding model factory: nil pool")
	}

	store := NewDBModelStore(pool)
	textModelID, err := store.ActiveModelIDByContentType(ctx, "text")
	if err != nil {
		return err
	}
	codeModelID, err := store.ActiveModelIDByContentType(ctx, "code")
	if err != nil {
		return err
	}
	summaryModelID, err := store.ActiveModelIDByTypeAndContent(ctx, "generation", "text")
	if err != nil {
		return err
	}
	if summaryModelID == nil {
		// Fallback: use the active embedding text model's provider profile.
		summaryModelID = textModelID
	}
	factory := NewModelEmbedderFactory(cfg, store)
	s.SetModelFactory(factory, textModelID, codeModelID, summaryModelID)
	return nil
}
