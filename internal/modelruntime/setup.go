package modelruntime

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/simplyblock/postbrain/internal/config"
	"github.com/simplyblock/postbrain/internal/modelstore"
	"github.com/simplyblock/postbrain/internal/providers"
)

// EnableModelDrivenFactory configures model-aware embedder and summarizer
// resolution on svc, loading the currently active model IDs from the DB.
func EnableModelDrivenFactory(ctx context.Context, svc *providers.EmbeddingService, pool *pgxpool.Pool, cfg *config.EmbeddingConfig) error {
	if svc == nil {
		return fmt.Errorf("embedding service is nil")
	}
	if pool == nil {
		return fmt.Errorf("embedding model factory: nil pool")
	}

	embStore := modelstore.NewEmbeddingModelStore(pool)
	genStore := modelstore.NewGenerationModelStore(pool)

	textModelID, err := embStore.ActiveModelIDByContentType(ctx, "text")
	if err != nil {
		return err
	}
	codeModelID, err := embStore.ActiveModelIDByContentType(ctx, "code")
	if err != nil {
		return err
	}
	summaryModelID, err := genStore.ActiveGenerationModelIDByContentType(ctx, "text")
	if err != nil {
		return err
	}
	if summaryModelID == nil {
		// Fallback: use the active embedding text model's provider profile.
		summaryModelID = textModelID
	}

	embFactory := NewEmbeddingFactory(cfg, embStore)
	sumFactory := NewSummaryFactory(cfg, genStore)
	svc.SetModelFactory(embFactory, sumFactory, textModelID, codeModelID, summaryModelID)
	return nil
}
