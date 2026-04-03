package postbraincli

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

const postbrainConfigMarker = "<!-- postbrain-config -->"

// InstallCodexSkill installs the postbrain codex skill file into targetDir and
// optionally appends a Postbrain hint block to AGENTS.md.
//
// Returns the installed skill path, whether AGENTS.md was updated, and an error.
func InstallCodexSkill(targetDir, skillContent, postbrainURL, postbrainScope string) (string, bool, error) {
	if strings.TrimSpace(targetDir) == "" {
		targetDir = "."
	}
	if strings.TrimSpace(skillContent) == "" {
		return "", false, fmt.Errorf("skill content is empty")
	}
	if strings.TrimSpace(postbrainURL) == "" {
		postbrainURL = "http://localhost:7433"
	}

	destDir := filepath.Join(targetDir, ".codex", "skills")
	destFile := filepath.Join(destDir, "postbrain.md")
	if err := os.MkdirAll(destDir, 0o755); err != nil {
		return "", false, fmt.Errorf("create destination directory: %w", err)
	}
	if err := os.WriteFile(destFile, []byte(skillContent), 0o644); err != nil {
		return "", false, fmt.Errorf("write skill file: %w", err)
	}

	agentsPath := filepath.Join(targetDir, "AGENTS.md")
	agentsBytes, err := os.ReadFile(agentsPath)
	if err != nil {
		if os.IsNotExist(err) {
			return destFile, false, nil
		}
		return "", false, fmt.Errorf("read AGENTS.md: %w", err)
	}
	if strings.Contains(string(agentsBytes), postbrainConfigMarker) {
		return destFile, false, nil
	}

	var block strings.Builder
	block.WriteString("\n")
	block.WriteString(postbrainConfigMarker)
	block.WriteString("\n## Postbrain\n\n")
	block.WriteString("The `.codex/skills/postbrain.md` skill is active for this project.\n\n")
	block.WriteString("```\n")
	block.WriteString("POSTBRAIN_URL=")
	block.WriteString(postbrainURL)
	block.WriteString("\n")
	if strings.TrimSpace(postbrainScope) != "" {
		block.WriteString("POSTBRAIN_SCOPE=")
		block.WriteString(postbrainScope)
		block.WriteString("\n")
	} else {
		block.WriteString("# POSTBRAIN_SCOPE=project:your-org/your-repo   <- set this to skip the scope prompt\n")
	}
	block.WriteString("```\n")

	f, err := os.OpenFile(agentsPath, os.O_APPEND|os.O_WRONLY, 0)
	if err != nil {
		return "", false, fmt.Errorf("open AGENTS.md for append: %w", err)
	}
	defer func() { _ = f.Close() }()
	if _, err := f.WriteString(block.String()); err != nil {
		return "", false, fmt.Errorf("append AGENTS.md postbrain block: %w", err)
	}

	return destFile, true, nil
}
