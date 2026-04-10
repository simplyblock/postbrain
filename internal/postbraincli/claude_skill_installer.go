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

	destDir := filepath.Join(targetDir, ".claude")
	destFile := filepath.Join(destDir, "postbrain.md")
	if err := os.MkdirAll(destDir, 0o755); err != nil {
		return "", false, fmt.Errorf("create destination directory: %w", err)
	}
	if err := os.WriteFile(destFile, []byte(skillContent), 0o644); err != nil {
		return "", false, fmt.Errorf("write skill file: %w", err)
	}
	if err := ensurePostbrainBaseFile(targetDir, ".claude", postbrainScope); err != nil {
		return "", false, err
	}

	claudePath := filepath.Join(targetDir, "CLAUDE.md")
	claudeBytes, err := os.ReadFile(claudePath)
	if err != nil {
		if os.IsNotExist(err) {
			return destFile, false, nil
		}
		return "", false, fmt.Errorf("read CLAUDE.md: %w", err)
	}
	if strings.Contains(string(claudeBytes), postbrainConfigMarker) {
		return destFile, false, nil
	}

	var block strings.Builder
	block.WriteString("\n")
	block.WriteString(postbrainConfigMarker)
	block.WriteString("\n## Postbrain\n\n")
	block.WriteString("@.claude/postbrain.md\n\n")
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

// InstallClaudeHooks merges Postbrain hooks into .claude/settings.json.
// It creates the file if it does not exist and preserves all existing settings.
// The call is idempotent: if postbrain hooks are already present, the file is
// not modified and updated=false is returned.
//
// If scope is non-empty it is inlined into the hook commands; otherwise the
// hooks reference $POSTBRAIN_SCOPE so the user can set it via an env var.
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
		return false, fmt.Errorf("read settings.json: %w", err)
	}
	if len(data) > 0 {
		if err := json.Unmarshal(data, &settings); err != nil {
			return false, fmt.Errorf("parse settings.json: %w", err)
		}
	}

	var snapshotCmd, summarizeCmd string
	if strings.TrimSpace(scope) == "" {
		scope = ResolveScopeFromBaseFiles(targetDir)
	}
	if strings.TrimSpace(scope) != "" {
		quotedScope := shellSingleQuote(scope)
		snapshotCmd = "postbrain-cli snapshot --scope " + quotedScope
		summarizeCmd = "postbrain-cli summarize-session --scope " + quotedScope
	} else {
		snapshotCmd = "postbrain-cli snapshot"
		summarizeCmd = "postbrain-cli summarize-session"
	}

	// Ensure the hooks map exists.
	hooks, _ := settings["hooks"].(map[string]any)
	if hooks == nil {
		hooks = make(map[string]any)
		settings["hooks"] = hooks
	}

	postToolUse, _ := hooks["PostToolUse"].([]any)
	stop, _ := hooks["Stop"].([]any)
	hasSnapshot := eventHooksContainCommand(postToolUse, "postbrain-cli snapshot")
	hasSummarize := eventHooksContainCommand(stop, "postbrain-cli summarize-session")
	if !hasSnapshot {
		// PostToolUse: snapshot on file edits.
		postToolUse = append(postToolUse, map[string]any{
			"matcher": "Edit|Write|Bash",
			"hooks": []any{
				map[string]any{
					"type":    "command",
					"command": snapshotCmd,
				},
			},
		})
		hooks["PostToolUse"] = postToolUse
	}
	if !hasSummarize {
		// Stop: summarize session when the agent stops.
		stop = append(stop, map[string]any{
			"matcher": "",
			"hooks": []any{
				map[string]any{
					"type":    "command",
					"command": summarizeCmd,
				},
			},
		})
		hooks["Stop"] = stop
	}
	if hasSnapshot && hasSummarize {
		return false, nil
	}

	out, err := json.MarshalIndent(settings, "", "  ")
	if err != nil {
		return false, fmt.Errorf("marshal settings.json: %w", err)
	}
	if err := os.WriteFile(settingsPath, append(out, '\n'), 0o644); err != nil {
		return false, fmt.Errorf("write settings.json: %w", err)
	}
	return true, nil
}
