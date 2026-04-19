package skills

import (
	"errors"
	"fmt"
	"regexp"
	"strings"

	"github.com/simplyblock/postbrain/internal/db"
)

// segmentRe matches a valid path segment: starts with alphanumeric, allows
// alphanumeric, dots, hyphens, underscores; max 255 chars per segment.
var segmentRe = regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9._-]{0,254}$`)

// ValidateSkillFile checks that a SkillFileInput is safe to store and install.
//
// Rules:
//  1. RelativePath must not be empty.
//  2. Must not equal "SKILL.md" (reserved for the main entry point).
//  3. Must not begin with "/" or "\" (no absolute paths).
//  4. Must not contain "\" anywhere (cross-platform safety).
//  5. Must not contain ".." as any path segment (no traversal).
//  6. Each segment must match ^[a-zA-Z0-9][a-zA-Z0-9._-]{0,254}$ (no hidden
//     files, no spaces, no control characters).
//  7. Total path length must not exceed 255 bytes.
//  8. Executable files (IsExecutable=true) must have paths prefixed "scripts/".
//  9. Files with a ".md" extension must have paths prefixed "references/".
func ValidateSkillFile(f db.SkillFileInput) error {
	p := f.RelativePath

	if p == "" {
		return errors.New("skills: file relative_path is empty")
	}
	if p == "SKILL.md" {
		return errors.New("skills: file relative_path \"SKILL.md\" is reserved for the main skill entry point")
	}
	if strings.HasPrefix(p, "/") || strings.HasPrefix(p, "\\") {
		return errors.New("skills: file relative_path must not be an absolute path")
	}
	if strings.Contains(p, "\\") {
		return errors.New("skills: file relative_path must not contain backslash characters")
	}
	if len(p) > 255 {
		return fmt.Errorf("skills: file relative_path is too long (%d bytes, max 255)", len(p))
	}

	segments := strings.Split(p, "/")
	for _, seg := range segments {
		if seg == ".." {
			return fmt.Errorf("skills: file relative_path %q contains traversal sequence \"..\"", p)
		}
		if !segmentRe.MatchString(seg) {
			return fmt.Errorf("skills: file relative_path %q contains invalid segment %q (must match ^[a-zA-Z0-9][a-zA-Z0-9._-]{0,254}$)", p, seg)
		}
	}

	// Subdirectory prefix enforcement.
	if f.IsExecutable && !strings.HasPrefix(p, "scripts/") {
		return fmt.Errorf("skills: executable file %q must have relative_path prefixed with scripts/", p)
	}
	if !f.IsExecutable && strings.HasSuffix(p, ".md") && !strings.HasPrefix(p, "references/") {
		return fmt.Errorf("skills: markdown file %q must have relative_path prefixed with references/", p)
	}

	return nil
}
