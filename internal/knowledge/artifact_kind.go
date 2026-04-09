package knowledge

import (
	"fmt"
	"strings"
)

const (
	ArtifactKindGeneral     = "general"
	ArtifactKindDecision    = "decision"
	ArtifactKindMeetingNote = "meeting_note"
	ArtifactKindRetro       = "retrospective"
	ArtifactKindSpec        = "spec"
	ArtifactKindDesignDoc   = "design_doc"
	ArtifactKindResearch    = "research"
)

var artifactKinds = []string{
	ArtifactKindGeneral,
	ArtifactKindDecision,
	ArtifactKindMeetingNote,
	ArtifactKindRetro,
	ArtifactKindSpec,
	ArtifactKindDesignDoc,
	ArtifactKindResearch,
}

var allowedArtifactKinds = func() map[string]struct{} {
	out := make(map[string]struct{}, len(artifactKinds))
	for _, kind := range artifactKinds {
		out[kind] = struct{}{}
	}
	return out
}()

// ArtifactKinds returns the supported artifact kind values in UI/API display order.
func ArtifactKinds() []string {
	out := make([]string, len(artifactKinds))
	copy(out, artifactKinds)
	return out
}

// NormalizeArtifactKind validates and normalizes artifact kind values.
// Empty kinds default to ArtifactKindGeneral.
func NormalizeArtifactKind(kind string) (string, error) {
	normalized := strings.TrimSpace(strings.ToLower(kind))
	if normalized == "" {
		return ArtifactKindGeneral, nil
	}
	if _, ok := allowedArtifactKinds[normalized]; !ok {
		return "", fmt.Errorf("knowledge: invalid artifact kind: %s", kind)
	}
	return normalized, nil
}
