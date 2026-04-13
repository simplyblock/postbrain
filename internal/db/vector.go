package db

import "fmt"

// ExportFloat32SliceToVector formats a []float32 as a pg_vector literal string.
// Kept for callers that build raw SQL.
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
