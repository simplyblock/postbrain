//go:build integration

package testhelper

import (
	"context"
	"math"
	"math/rand/v2"

	"github.com/simplyblock/postbrain/internal/embedding"
)

// NewMockEmbeddingService returns an EmbeddingService backed by a mock embedder
// that returns random 4-dimensional vectors. Use 4 dims to match CreateTestEmbeddingModel.
func NewMockEmbeddingService() *embedding.EmbeddingService {
	return embedding.NewServiceFromEmbedders(&mockEmbedder{}, nil)
}

type mockEmbedder struct{}

func (m *mockEmbedder) Embed(_ context.Context, _ string) ([]float32, error) {
	return []float32{rand.Float32(), rand.Float32(), rand.Float32(), rand.Float32()}, nil
}

func (m *mockEmbedder) EmbedBatch(_ context.Context, texts []string) ([][]float32, error) {
	out := make([][]float32, len(texts))
	for i := range texts {
		out[i] = []float32{rand.Float32(), rand.Float32(), rand.Float32(), rand.Float32()}
	}
	return out, nil
}

func (m *mockEmbedder) ModelSlug() string { return "test/model" }
func (m *mockEmbedder) Dimensions() int   { return 4 }

// NewDeterministicEmbeddingService returns a service that returns the same vector
// for the same input string (seeded hash-based). Identical content produces identical
// vectors, which means cosine distance = 0 and near-duplicate detection fires.
func NewDeterministicEmbeddingService() *embedding.EmbeddingService {
	return embedding.NewServiceFromEmbedders(&deterministicEmbedder{}, nil)
}

type deterministicEmbedder struct{}

func (d *deterministicEmbedder) Embed(_ context.Context, text string) ([]float32, error) {
	h := fnv32(text)
	v := []float32{
		float32(h&0xFF) / 255.0,
		float32((h>>8)&0xFF) / 255.0,
		float32((h>>16)&0xFF) / 255.0,
		float32((h>>24)&0xFF) / 255.0,
	}
	return normalizeVec(v), nil
}

func (d *deterministicEmbedder) EmbedBatch(_ context.Context, texts []string) ([][]float32, error) {
	out := make([][]float32, len(texts))
	for i, t := range texts {
		v, _ := d.Embed(context.Background(), t)
		out[i] = v
	}
	return out, nil
}

func (d *deterministicEmbedder) ModelSlug() string { return "test/model" }
func (d *deterministicEmbedder) Dimensions() int   { return 4 }

func fnv32(s string) uint32 {
	h := uint32(2166136261)
	for _, c := range []byte(s) {
		h ^= uint32(c)
		h *= 16777619
	}
	return h
}

func normalizeVec(v []float32) []float32 {
	var sum float32
	for _, x := range v {
		sum += x * x
	}
	if sum == 0 {
		return v
	}
	sq := float32(math.Sqrt(float64(sum)))
	out := make([]float32, len(v))
	for i, x := range v {
		out[i] = x / sq
	}
	return out
}
