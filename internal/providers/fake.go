package providers

import (
	"context"
	"hash/fnv"
	"math"
)

const (
	// FakeModelSlug is the ModelSlug returned by FakeEmbedder.
	FakeModelSlug = "fake-embedder"
	// FakeModelDims is the default number of dimensions produced by FakeEmbedder.
	FakeModelDims = 384
)

// FakeEmbedder is a deterministic, dependency-free Embedder for use in tests.
//
// It produces a unique unit vector for each distinct input text by seeding a
// fast PRNG (xorshift64) from the FNV-1a hash of the text. Properties:
//
//   - Deterministic: the same text always returns the same vector.
//   - Unique: different texts almost always produce different vectors.
//   - Normalized: vectors have unit L2 norm so cosine-similarity comparisons work.
//   - Configurable: dimensions can be set to match whatever the test DB expects.
//
// Use NewFakeEmbedder to construct one; the zero value is not valid.
type FakeEmbedder struct {
	dims int
}

// NewFakeEmbedder returns a FakeEmbedder that produces dims-dimensional vectors.
// Pass 0 to accept the default (FakeModelDims = 384).
func NewFakeEmbedder(dims int) *FakeEmbedder {
	if dims <= 0 {
		dims = FakeModelDims
	}
	return &FakeEmbedder{dims: dims}
}

// Embed returns a deterministic unit vector for text.
func (f *FakeEmbedder) Embed(_ context.Context, text string) ([]float32, error) {
	return deterministicUnitVector(text, f.dims), nil
}

// EmbedBatch embeds each text independently.
func (f *FakeEmbedder) EmbedBatch(_ context.Context, texts []string) ([][]float32, error) {
	out := make([][]float32, len(texts))
	for i, t := range texts {
		out[i] = deterministicUnitVector(t, f.dims)
	}
	return out, nil
}

// ModelSlug returns FakeModelSlug.
func (f *FakeEmbedder) ModelSlug() string { return FakeModelSlug }

// Dimensions returns the configured number of dimensions.
func (f *FakeEmbedder) Dimensions() int { return f.dims }

// deterministicUnitVector generates a reproducible unit vector in `dims`
// dimensions from text. It seeds xorshift64 with the FNV-1a hash and steps
// the PRNG once per dimension, then normalizes the result.
func deterministicUnitVector(text string, dims int) []float32 {
	h := fnv.New64a()
	_, _ = h.Write([]byte(text))
	state := h.Sum64()

	vec := make([]float32, dims)
	var norm float64
	for i := range vec {
		state = xorshift64(state)
		// Map the unsigned 64-bit value to [-1, 1].
		v := float64(int64(state)) / (1 << 63)
		vec[i] = float32(v)
		norm += v * v
	}
	if norm > 0 {
		inv := float32(1.0 / math.Sqrt(norm))
		for i := range vec {
			vec[i] *= inv
		}
	}
	return vec
}

// xorshift64 is a fast, bijective 64-bit pseudo-random step.
func xorshift64(x uint64) uint64 {
	if x == 0 {
		x = 0xcafebabe // avoid the all-zeros fixed point
	}
	x ^= x << 13
	x ^= x >> 7
	x ^= x << 17
	return x
}
