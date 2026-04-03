package postbraincli

import (
	"strconv"
	"strings"
)

// ReadSkillVersion parses the version field from a YAML frontmatter block at
// the start of a skill file's content. Returns 0 if the content has no
// frontmatter or no version field.
func ReadSkillVersion(content string) int {
	lines := strings.SplitN(content, "\n", 50)
	if len(lines) == 0 || strings.TrimSpace(lines[0]) != "---" {
		return 0
	}
	for _, line := range lines[1:] {
		if strings.TrimSpace(line) == "---" {
			break
		}
		if strings.HasPrefix(line, "version:") {
			parts := strings.SplitN(line, ":", 2)
			if len(parts) == 2 {
				if v, err := strconv.Atoi(strings.TrimSpace(parts[1])); err == nil {
					return v
				}
			}
		}
	}
	return 0
}