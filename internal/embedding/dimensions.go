package embedding

// FitDimensions returns a vector resized to dims.
// If dims <= 0, vec is returned unchanged.
// If vec is longer than dims, it is truncated.
// If vec is shorter than dims, it is zero-padded.
func FitDimensions(vec []float32, dims int) []float32 {
	if dims <= 0 {
		return vec
	}
	if len(vec) == dims {
		return vec
	}
	if len(vec) > dims {
		return vec[:dims]
	}
	out := make([]float32, dims)
	copy(out, vec)
	return out
}
