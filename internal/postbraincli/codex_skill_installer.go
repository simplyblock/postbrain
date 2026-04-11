package postbraincli

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

const postbrainConfigMarker = "<!-- postbrain-config -->"

// CodexSkillInstallOptions controls optional Codex installer behavior.
type CodexSkillInstallOptions struct {
	InstallHooks bool
}

// InstallCodexSkill installs the postbrain codex skill file into targetDir and
// optionally appends a Postbrain hint block to AGENTS.md.
//
// Returns the installed skill path, whether AGENTS.md was updated, and an error.
func InstallCodexSkill(targetDir, skillContent, postbrainURL, postbrainScope string) (string, bool, error) {
	return InstallCodexSkillWithOptions(targetDir, skillContent, postbrainURL, postbrainScope, CodexSkillInstallOptions{
		InstallHooks: true,
	})
}

// InstallCodexSkillWithOptions installs the postbrain codex skill file with
// optional hook provisioning based on install options.
func InstallCodexSkillWithOptions(
	targetDir, skillContent, postbrainURL, postbrainScope string,
	opts CodexSkillInstallOptions,
) (string, bool, error) {
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
	if err := ensurePostbrainBaseFile(targetDir, ".codex", postbrainScope); err != nil {
		return "", false, err
	}
	if opts.InstallHooks {
		if _, err := InstallCodexHooks(targetDir, postbrainScope); err != nil {
			return "", false, err
		}
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

// InstallCodexHooks merges Postbrain hooks into .codex/hooks.json.
// It creates the file if it does not exist and preserves existing settings.
// The call is idempotent.
func InstallCodexHooks(targetDir, scope string) (bool, error) {
	if strings.TrimSpace(targetDir) == "" {
		targetDir = "."
	}

	configDir := filepath.Join(targetDir, ".codex")
	hooksPath := filepath.Join(configDir, "hooks.json")
	if err := os.MkdirAll(configDir, 0o755); err != nil {
		return false, fmt.Errorf("create .codex directory: %w", err)
	}

	root := make(map[string]any)
	data, err := os.ReadFile(hooksPath)
	if err != nil && !os.IsNotExist(err) {
		return false, fmt.Errorf("read hooks.json: %w", err)
	}
	trimmedData := strings.TrimSpace(string(data))
	if trimmedData != "" {
		if err := json.Unmarshal([]byte(trimmedData), &root); err != nil {
			return false, fmt.Errorf("parse hooks.json: %w", err)
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

	hooks, _ := root["hooks"].(map[string]any)
	if hooks == nil {
		hooks = make(map[string]any)
		root["hooks"] = hooks
	}

	postToolUse, _ := hooks["PostToolUse"].([]any)
	stop, _ := hooks["Stop"].([]any)

	postToolUse, snapshotUpdated := ensureEventHookCommand(
		postToolUse,
		"postbrain-cli snapshot",
		snapshotCmd,
		map[string]any{"matcher": "Bash"},
	)
	stop, summarizeUpdated := ensureEventHookCommand(
		stop,
		"postbrain-cli summarize-session",
		summarizeCmd,
		nil,
	)

	hooks["PostToolUse"] = postToolUse
	hooks["Stop"] = stop

	if !snapshotUpdated && !summarizeUpdated {
		return false, nil
	}

	var out bytes.Buffer
	enc := json.NewEncoder(&out)
	enc.SetIndent("", "  ")
	enc.SetEscapeHTML(false)
	if err := enc.Encode(root); err != nil {
		return false, fmt.Errorf("marshal hooks.json: %w", err)
	}
	if err := os.WriteFile(hooksPath, out.Bytes(), 0o644); err != nil {
		return false, fmt.Errorf("write hooks.json: %w", err)
	}
	return true, nil
}

// EnableCodexHooks ensures .codex/config.toml enables experimental Codex hooks
// via [features].codex_hooks = true. The operation is idempotent.
func EnableCodexHooks(targetDir string) (bool, error) {
	if strings.TrimSpace(targetDir) == "" {
		targetDir = "."
	}

	configDir := filepath.Join(targetDir, ".codex")
	configPath := filepath.Join(configDir, "config.toml")
	if err := os.MkdirAll(configDir, 0o755); err != nil {
		return false, fmt.Errorf("create .codex directory: %w", err)
	}

	data, err := os.ReadFile(configPath)
	if err != nil && !os.IsNotExist(err) {
		return false, fmt.Errorf("read config.toml: %w", err)
	}

	updated, content := ensureCodexHooksEnabled(string(data))
	if !updated {
		return false, nil
	}
	if !strings.HasSuffix(content, "\n") {
		content += "\n"
	}
	if err := os.WriteFile(configPath, []byte(content), 0o644); err != nil {
		return false, fmt.Errorf("write config.toml: %w", err)
	}
	return true, nil
}

func ensureCodexHooksEnabled(content string) (bool, string) {
	if strings.TrimSpace(content) == "" {
		return true, "[features]\ncodex_hooks = true\n"
	}

	lines := strings.Split(content, "\n")
	featuresStart, featuresEnd := findSection(lines, "features")
	if featuresStart >= 0 {
		for i := featuresStart + 1; i < featuresEnd; i++ {
			key, value, ok := parseTOMLAssignment(lines[i])
			if !ok || key != "codex_hooks" {
				continue
			}
			if strings.EqualFold(strings.TrimSpace(value), "true") {
				return false, content
			}
			lines[i] = "codex_hooks = true"
			return true, strings.Join(lines, "\n")
		}

		lines = append(lines, "")
		copy(lines[featuresEnd+1:], lines[featuresEnd:])
		lines[featuresEnd] = "codex_hooks = true"
		return true, strings.Join(lines, "\n")
	}

	trimmed := strings.TrimRight(content, "\n")
	if trimmed == "" {
		return true, "[features]\ncodex_hooks = true\n"
	}
	return true, trimmed + "\n\n[features]\ncodex_hooks = true\n"
}

func findSection(lines []string, section string) (start, end int) {
	start = -1
	end = len(lines)
	target := "[" + section + "]"
	for i, line := range lines {
		trimmed := strings.TrimSpace(line)
		if !isSectionHeader(trimmed) {
			continue
		}
		if start >= 0 {
			end = i
			return start, end
		}
		if strings.EqualFold(trimmed, target) {
			start = i
		}
	}
	return start, end
}

func isSectionHeader(line string) bool {
	return strings.HasPrefix(line, "[") && strings.HasSuffix(line, "]")
}

func parseTOMLAssignment(line string) (key, value string, ok bool) {
	trimmed := strings.TrimSpace(line)
	if trimmed == "" || strings.HasPrefix(trimmed, "#") {
		return "", "", false
	}
	eq := strings.Index(trimmed, "=")
	if eq <= 0 {
		return "", "", false
	}
	key = strings.TrimSpace(trimmed[:eq])
	value = strings.TrimSpace(trimmed[eq+1:])
	return key, value, true
}

func ensureEventHookCommand(entries []any, needle, desiredCommand string, entryDefaults map[string]any) ([]any, bool) {
	updated := false
	found := false
	for _, e := range entries {
		entry, _ := e.(map[string]any)
		hooksList, _ := entry["hooks"].([]any)
		for _, h := range hooksList {
			hook, _ := h.(map[string]any)
			cmd, _ := hook["command"].(string)
			if strings.Contains(cmd, needle) {
				found = true
				if cmd != desiredCommand {
					hook["command"] = desiredCommand
					updated = true
				}
			}
		}
	}
	if found {
		return entries, updated
	}

	newEntry := map[string]any{
		"hooks": []any{
			map[string]any{
				"type":    "command",
				"command": desiredCommand,
			},
		},
	}
	for k, v := range entryDefaults {
		newEntry[k] = v
	}
	return append(entries, newEntry), true
}
