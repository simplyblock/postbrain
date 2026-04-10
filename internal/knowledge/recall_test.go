package knowledge

import (
	"math"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/simplyblock/postbrain/internal/db"
)

func TestRecallScore_KnowledgeBoost(t *testing.T) {
	// With w_vec=0.50, w_bm25=0.10, w_trgm=0.10, w_imp=0.20, w_rec=0.10, plus +0.1 boost.
	vecScore := 0.8
	bm25Score := 0.0
	trgmScore := 0.0
	importance := 0.5 // 5 endorsements / 10
	recency := 1.0

	base := 0.50*vecScore + 0.10*bm25Score + 0.10*trgmScore + 0.20*importance + 0.10*recency
	expected := base + 0.10

	got := knowledgeCombinedScore(vecScore, bm25Score, trgmScore, importance, recency)
	if math.Abs(got-expected) > 1e-9 {
		t.Fatalf("expected %v, got %v", expected, got)
	}
}

func TestNormalizeEndorsements(t *testing.T) {
	tests := []struct {
		count    int
		expected float64
	}{
		{0, 0.0},
		{5, 0.5},
		{10, 1.0},
		{20, 1.0}, // capped
	}
	for _, tt := range tests {
		got := normalizeEndorsements(tt.count)
		if math.Abs(got-tt.expected) > 1e-9 {
			t.Fatalf("normalizeEndorsements(%d) = %v, want %v", tt.count, got, tt.expected)
		}
	}
}

func TestArtifactKindQueryBoost_DecisionIntentPrefersDecisionArtifacts(t *testing.T) {
	t.Parallel()
	query := "why did we choose this architecture?"
	decisionBoost := artifactKindQueryBoost(query, ArtifactKindDecision)
	meetingBoost := artifactKindQueryBoost(query, ArtifactKindMeetingNote)
	if decisionBoost <= meetingBoost {
		t.Fatalf("decision boost (%v) must be greater than meeting note boost (%v)", decisionBoost, meetingBoost)
	}
}

func TestArtifactRecencyScore_MeetingNotesDecayFasterThanDecisions(t *testing.T) {
	t.Parallel()
	now := time.Date(2026, 4, 9, 12, 0, 0, 0, time.UTC)
	tenDaysAgo := now.Add(-10 * 24 * time.Hour)
	meeting := artifactRecencyScore(now, nil, tenDaysAgo, ArtifactKindMeetingNote)
	decision := artifactRecencyScore(now, nil, tenDaysAgo, ArtifactKindDecision)
	if meeting >= decision {
		t.Fatalf("meeting recency (%v) must be lower than decision recency (%v) at equal age", meeting, decision)
	}
}

func TestArtifactKindQueryBoost_DoesNotMatchHowInsideShow(t *testing.T) {
	t.Parallel()
	boost := artifactKindQueryBoost("show me status", ArtifactKindSpec)
	if boost != 0 {
		t.Fatalf("expected no implementation boost from 'show', got %v", boost)
	}
}

func TestArtifactWindowTimestamp_PrefersPublishedAt(t *testing.T) {
	t.Parallel()
	created := time.Date(2026, 4, 1, 10, 0, 0, 0, time.UTC)
	published := time.Date(2026, 4, 7, 11, 0, 0, 0, time.UTC)
	a := &db.KnowledgeArtifact{
		ID:          uuid.New(),
		CreatedAt:   created,
		PublishedAt: &published,
	}
	got := artifactWindowTimestamp(a)
	if !got.Equal(published) {
		t.Fatalf("artifactWindowTimestamp=%s, want %s", got, published)
	}
}

func TestArtifactWithinWindow_UsesPublishedAtWhenPresent(t *testing.T) {
	t.Parallel()
	created := time.Date(2026, 4, 1, 10, 0, 0, 0, time.UTC)
	published := time.Date(2026, 4, 7, 11, 0, 0, 0, time.UTC)
	a := &db.KnowledgeArtifact{
		ID:          uuid.New(),
		CreatedAt:   created,
		PublishedAt: &published,
	}
	since := time.Date(2026, 4, 5, 0, 0, 0, 0, time.UTC)
	if !artifactWithinWindow(a, &since, nil) {
		t.Fatal("expected artifact to pass since-window using published_at")
	}
}

func TestArtifactWithinWindow_FallsBackToCreatedAtWhenUnpublished(t *testing.T) {
	t.Parallel()
	created := time.Date(2026, 4, 1, 10, 0, 0, 0, time.UTC)
	a := &db.KnowledgeArtifact{
		ID:        uuid.New(),
		CreatedAt: created,
	}
	since := time.Date(2026, 4, 5, 0, 0, 0, 0, time.UTC)
	if artifactWithinWindow(a, &since, nil) {
		t.Fatal("expected artifact to fail since-window using created_at fallback")
	}
}
