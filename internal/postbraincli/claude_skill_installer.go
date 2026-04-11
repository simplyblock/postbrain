package postbraincli

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// InstallClaudeSkill installs the postbrain Claude Code instructions file into
// targetDir and optionally appends a Postbrain hint block to CLAUDE.md.
//
// Returns the installed skill path, whether CLAUDE.md was updated, and an error.
func InstallClaudeSkill(targetDir, skillContent, postbrainURL, postbrainScope string) (string, bool, error) {
	if strings.TrimSpace(targetDir) == "" {
		targetDir = "."
	}
	if strings.TrimSpace(skillContent) == "" {
		return "", false, fmt.Errorf("skill content is empty")
	}
	if strings.TrimSpace(postbrainURL) == "" {
		postbrainURL = "http://localhost:7433"
	}

	destDir := filepath.Join(targetDir, ".claude", "skills", "postbrain")
	destFile := filepath.Join(destDir, "SKILL.md")
	legacyFile := filepath.Join(targetDir, ".claude", "postbrain.md")
	if err := os.MkdirAll(destDir, 0o755); err != nil {
		return "", false, fmt.Errorf("create destination directory: %w", err)
	}
	if err := os.WriteFile(destFile, []byte(skillContent), 0o644); err != nil {
		return "", false, fmt.Errorf("write skill file: %w", err)
	}
	if err := os.Remove(legacyFile); err != nil && !os.IsNotExist(err) {
		return "", false, fmt.Errorf("remove legacy skill file: %w", err)
	}
	if err := ensurePostbrainBaseFile(targetDir, ".claude", postbrainScope); err != nil {
		return "", false, err
	}

	claudePath := filepath.Join(targetDir, "CLAUDE.md")
	claudeBytes, err := os.ReadFile(claudePath)
	if err != nil {
		if os.IsNotExist(err) {
			claudeBytes = nil
		} else {
			return "", false, fmt.Errorf("read CLAUDE.md: %w", err)
		}
	}
	if strings.Contains(string(claudeBytes), postbrainConfigMarker) {
		return destFile, false, nil
	}

	var block strings.Builder
	block.WriteString("\n")
	block.WriteString(postbrainConfigMarker)
	block.WriteString("\n## Postbrain\n\n")
	block.WriteString("@.claude/skills/postbrain/SKILL.md\n\n")
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

	if len(claudeBytes) == 0 {
		initial := "# Project\n"
		if err := os.WriteFile(claudePath, []byte(initial+block.String()), 0o644); err != nil {
			return "", false, fmt.Errorf("write CLAUDE.md: %w", err)
		}
		return destFile, true, nil
	}
	f, err := os.OpenFile(claudePath, os.O_APPEND|os.O_WRONLY, 0)
	if err != nil {
		return "", false, fmt.Errorf("open CLAUDE.md for append: %w", err)
	}
	defer func() { _ = f.Close() }()
	if _, err := f.WriteString(block.String()); err != nil {
		return "", false, fmt.Errorf("append CLAUDE.md postbrain block: %w", err)
	}

	return destFile, true, nil
}

// InstallClaudeHooks merges Postbrain hooks into .claude/settings.local.json.
// It creates the file if it does not exist and preserves all existing settings.
// The call is idempotent: if postbrain hooks are already present, the file is
// not modified and updated=false is returned.
//
// Hook commands are intentionally installed without explicit scope flags so
// runtime scope resolution inside `postbrain-cli` is always used.
func InstallClaudeHooks(targetDir, scope string) (bool, error) {
	if strings.TrimSpace(targetDir) == "" {
		targetDir = "."
	}

	claudeDir := filepath.Join(targetDir, ".claude")
	settingsPath := filepath.Join(claudeDir, "settings.local.json")
	if err := os.MkdirAll(claudeDir, 0o755); err != nil {
		return false, fmt.Errorf("create .claude directory: %w", err)
	}

	// Read existing settings or start from an empty map.
	settings := make(map[string]any)
	data, err := os.ReadFile(settingsPath)
	if err != nil && !os.IsNotExist(err) {
		return false, fmt.Errorf("read settings.local.json: %w", err)
	}
	if len(data) > 0 {
		if err := json.Unmarshal(data, &settings); err != nil {
			return false, fmt.Errorf("parse settings.local.json: %w", err)
		}
	}

	snapshotCmd := "postbrain-cli snapshot"
	summarizeCmd := "postbrain-cli summarize-session"

	// Ensure the hooks map exists.
	hooks, _ := settings["hooks"].(map[string]any)
	if hooks == nil {
		hooks = make(map[string]any)
		settings["hooks"] = hooks
	}

	postToolUse, _ := hooks["PostToolUse"].([]any)
	stop, _ := hooks["Stop"].([]any)

	postToolUse, snapshotUpdated := ensureEventHookCommand(
		postToolUse,
		"postbrain-cli snapshot",
		snapshotCmd,
		map[string]any{"matcher": "Edit|Write|Bash"},
	)
	stop, summarizeUpdated := ensureEventHookCommand(
		stop,
		"postbrain-cli summarize-session",
		summarizeCmd,
		map[string]any{"matcher": ""},
	)

	hooks["PostToolUse"] = postToolUse
	hooks["Stop"] = stop
	if !snapshotUpdated && !summarizeUpdated {
		return false, nil
	}

	out, err := json.MarshalIndent(settings, "", "  ")
	if err != nil {
		return false, fmt.Errorf("marshal settings.local.json: %w", err)
	}
	if err := os.WriteFile(settingsPath, append(out, '\n'), 0o644); err != nil {
		return false, fmt.Errorf("write settings.local.json: %w", err)
	}
	return true, nil
}
