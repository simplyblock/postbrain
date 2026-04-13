// Package compat exposes the legacy free-function API that existing callers
// depend on. Each function creates a Queries value from the pool and delegates
// to the generated method.
package compat

import (
	"fmt"
	"time"

	"github.com/google/uuid"
	pgvector "github.com/pgvector/pgvector-go"
)

var maxRecallTime = time.Date(9999, 12, 31, 23, 59, 59, 0, time.UTC)

func normalizeRecallWindowBounds(since, until *time.Time) (time.Time, time.Time) {
	lower := time.Time{}
	upper := maxRecallTime
	if since != nil {
		lower = since.UTC()
	}
	if until != nil {
		upper = until.UTC()
	}
	return lower, upper
}

// ExportFloat32SliceToVector formats a []float32 as a pg_vector literal string.
// Kept for backward compatibility with callers that build raw SQL.
func ExportFloat32SliceToVector(v []float32) string { return float32SliceToVector(v) }

// float32SliceToVector formats a []float32 as a pg_vector literal string, e.g. "[1,2,3]".
func float32SliceToVector(v []float32) string {
	if len(v) == 0 {
		return "[]"
	}
	b := make([]byte, 0, len(v)*8+2)
	b = append(b, '[')
	for i, f := range v {
		if i > 0 {
			b = append(b, ',')
		}
		b = fmt.Appendf(b, "%g", f)
	}
	b = append(b, ']')
	return string(b)
}

// vecPtr returns a *pgvector.Vector when the slice is non-empty, or nil (SQL NULL)
// when the slice is empty.
func vecPtr(vec []float32) *pgvector.Vector {
	if len(vec) == 0 {
		return nil
	}
	v := pgvector.NewVector(vec)
	return &v
}

func nilIfZeroUUID(id *uuid.UUID) *uuid.UUID {
	if id == nil {
		return nil
	}
	if *id == uuid.Nil {
		return nil
	}
	return id
}
