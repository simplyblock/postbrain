package jobs

import (
	"context"
	"fmt"
)

// RunPaginatedBatch fetches items in offset-based pages and calls process for
// each item until an empty page is returned or fewer than batchSize items are
// fetched. process receives each item; errors must be handled inside the
// closure. Returns the total number of items fetched and the first fetch error.
func RunPaginatedBatch[T any](
	ctx context.Context,
	batchSize int,
	fetch func(context.Context, int, int) ([]T, error),
	process func(context.Context, T),
) (int, error) {
	offset, total := 0, 0
	for {
		batch, err := fetch(ctx, batchSize, offset)
		if err != nil {
			return total, fmt.Errorf("offset %d: %w", offset, err)
		}
		if len(batch) == 0 {
			break
		}
		for _, item := range batch {
			process(ctx, item)
		}
		total += len(batch)
		if len(batch) < batchSize {
			break
		}
		offset += batchSize
	}
	return total, nil
}
