package embedding

import (
	"context"
	"math"
	"testing"
)

func TestFakeEmbedder_Deterministic(t *testing.T) {
	t.Parallel()
	f := NewFakeEmbedder(0)
	ctx := context.Background()

	a, err := f.Embed(ctx, "hello world")
	if err != nil {
		t.Fatalf("Embed: %v", err)
	}
	b, err := f.Embed(ctx, "hello world")
	if err != nil {
		t.Fatalf("Embed second call: %v", err)
	}
	for i := range a {
		if a[i] != b[i] {
			t.Fatalf("not deterministic: a[%d]=%v b[%d]=%v", i, a[i], i, b[i])
		}
	}
}

func TestFakeEmbedder_DifferentTexts(t *testing.T) {
	t.Parallel()
	f := NewFakeEmbedder(0)
	ctx := context.Background()

	a, _ := f.Embed(ctx, "apple")
	b, _ := f.Embed(ctx, "orange")

	same := true
	for i := range a {
		if a[i] != b[i] {
			same = false
			break
		}
	}
	if same {
		t.Error("different texts produced identical vectors")
	}
}

func TestFakeEmbedder_UnitLength(t *testing.T) {
	t.Parallel()
	f := NewFakeEmbedder(0)
	ctx := context.Background()

	for _, text := range []string{"", "hello", "the quick brown fox", "日本語テスト"} {
		vec, err := f.Embed(ctx, text)
		if err != nil {
			t.Fatalf("Embed(%q): %v", text, err)
		}
		var norm float64
		for _, v := range vec {
			norm += float64(v) * float64(v)
		}
		norm = math.Sqrt(norm)
		if math.Abs(norm-1.0) > 1e-5 {
			t.Errorf("text=%q: L2 norm = %v, want 1.0", text, norm)
		}
	}
}

func TestFakeEmbedder_DefaultDims(t *testing.T) {
	t.Parallel()
	f := NewFakeEmbedder(0)
	if f.Dimensions() != FakeModelDims {
		t.Errorf("Dimensions() = %d, want %d", f.Dimensions(), FakeModelDims)
	}
	vec, _ := f.Embed(context.Background(), "test")
	if len(vec) != FakeModelDims {
		t.Errorf("len(vec) = %d, want %d", len(vec), FakeModelDims)
	}
}

func TestFakeEmbedder_CustomDims(t *testing.T) {
	t.Parallel()
	f := NewFakeEmbedder(16)
	if f.Dimensions() != 16 {
		t.Errorf("Dimensions() = %d, want 16", f.Dimensions())
	}
	vec, _ := f.Embed(context.Background(), "test")
	if len(vec) != 16 {
		t.Errorf("len(vec) = %d, want 16", len(vec))
	}
}

func TestFakeEmbedder_EmbedBatch(t *testing.T) {
	t.Parallel()
	f := NewFakeEmbedder(8)
	ctx := context.Background()
	texts := []string{"alpha", "beta", "gamma"}

	batch, err := f.EmbedBatch(ctx, texts)
	if err != nil {
		t.Fatalf("EmbedBatch: %v", err)
	}
	if len(batch) != len(texts) {
		t.Fatalf("len(batch) = %d, want %d", len(batch), len(texts))
	}
	// Each batch result must equal the single-embed result.
	for i, text := range texts {
		single, _ := f.Embed(ctx, text)
		for j := range single {
			if batch[i][j] != single[j] {
				t.Errorf("batch[%d][%d] = %v, want %v", i, j, batch[i][j], single[j])
			}
		}
	}
}

func TestFakeEmbedder_ModelSlug(t *testing.T) {
	t.Parallel()
	f := NewFakeEmbedder(0)
	if f.ModelSlug() != FakeModelSlug {
		t.Errorf("ModelSlug() = %q, want %q", f.ModelSlug(), FakeModelSlug)
	}
}
