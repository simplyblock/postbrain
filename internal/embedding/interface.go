package embedding

import "context"

// Embedder produces dense vector embeddings from text.
type Embedder interface {
	Embed(ctx context.Context, text string) ([]float32, error)
	EmbedBatch(ctx context.Context, texts []string) ([][]float32, error)
	ModelSlug() string
	Dimensions() int
}
