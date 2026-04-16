package skills

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/simplyblock/postbrain/internal/db"
)

// slugRe matches valid skill slugs: lowercase alphanumeric, hyphens and
// underscores, starting with an alphanumeric character, max 64 chars.
// This pattern intentionally excludes dots, slashes, backslashes, spaces and
// uppercase so that slugs can never be used as path-traversal payloads.
var slugRe = regexp.MustCompile(`^[a-z0-9][a-z0-9_-]{0,63}$`)

// ValidateSlug returns an error if slug would be unsafe to use as a filename
// component. The regex rejects traversal sequences (../, /, \), dots, spaces,
// uppercase letters, leading dashes/underscores, and empty strings.
func ValidateSlug(slug string) error {
	if !slugRe.MatchString(slug) {
		return errors.New("skills: slug must match ^[a-z0-9][a-z0-9_-]{0,63}$ (no dots, slashes, spaces, uppercase)")
	}
	return nil
}

// TargetPath returns the absolute path where a skill would be installed.
// "codex" agent type installs to {workdir}/.codex/skills/{slug}.md.
// All other agent types install to {workdir}/.claude/commands/{slug}.md.
func TargetPath(slug, agentType, workdir string) string {
	if agentType == "codex" {
		return filepath.Join(workdir, ".codex", "skills", slug+".md")
	}
	return filepath.Join(workdir, ".claude", "commands", slug+".md")
}

// IsInstalled reports whether the skill file exists at the expected path.
func IsInstalled(slug, agentType, workdir string) bool {
	_, err := os.Stat(TargetPath(slug, agentType, workdir))
	return err == nil
}

// Install materialises a skill to the agent's command directory and returns
// the absolute path of the written file.
func Install(skill *db.Skill, agentType, workdir string) (string, error) {
	if err := ValidateSlug(skill.Slug); err != nil {
		return "", err
	}

	target := TargetPath(skill.Slug, agentType, workdir)

	// Create the directory first so EvalSymlinks can resolve it below.
	if err := os.MkdirAll(filepath.Dir(target), 0755); err != nil {
		return "", fmt.Errorf("skills: install mkdir: %w", err)
	}

	// Defense-in-depth: resolve symlinks on both paths before comparing so that
	// a symlink under workdir (e.g. workdir/.claude → /etc) cannot be used to
	// escape the intended base directory.  filepath.Abs alone does not protect
	// against this — filepath.EvalSymlinks follows every symlink component.
	realBase, err := filepath.EvalSymlinks(workdir)
	if err != nil {
		return "", fmt.Errorf("skills: install resolve base: %w", err)
	}
	// The target file itself does not exist yet; resolve its parent (which was
	// just created by MkdirAll) and re-attach the filename.
	realDir, err := filepath.EvalSymlinks(filepath.Dir(target))
	if err != nil {
		return "", fmt.Errorf("skills: install resolve target dir: %w", err)
	}
	realTarget := filepath.Join(realDir, filepath.Base(target))
	if !strings.HasPrefix(realTarget, filepath.Clean(realBase)+string(filepath.Separator)) {
		return "", fmt.Errorf("skills: install path %q escapes base directory %q", realTarget, realBase)
	}

	agentTypesJSON, err := json.Marshal(skill.AgentTypes)
	if err != nil {
		return "", fmt.Errorf("skills: install marshal agent_types: %w", err)
	}

	var sb strings.Builder
	sb.WriteString("---\n")
	sb.WriteString(fmt.Sprintf("name: %s\n", skill.Name))
	sb.WriteString(fmt.Sprintf("description: %s\n", skill.Description))
	sb.WriteString(fmt.Sprintf("agent_types: %s\n", string(agentTypesJSON)))
	sb.WriteString(fmt.Sprintf("version: %d\n", skill.Version))

	// Write parameters if present.
	if len(skill.Parameters) > 0 && string(skill.Parameters) != "[]" && string(skill.Parameters) != "null" {
		var params []db.SkillParameter
		if err := json.Unmarshal(skill.Parameters, &params); err == nil && len(params) > 0 {
			sb.WriteString("parameters:\n")
			for _, p := range params {
				sb.WriteString(fmt.Sprintf("  - name: %s\n", p.Name))
				sb.WriteString(fmt.Sprintf("    type: %s\n", p.Type))
				sb.WriteString(fmt.Sprintf("    required: %v\n", p.Required))
				if p.Description != "" {
					sb.WriteString(fmt.Sprintf("    description: %s\n", p.Description))
				}
				if len(p.Values) > 0 {
					sb.WriteString(fmt.Sprintf("    values: %v\n", p.Values))
				}
			}
		}
	}
	sb.WriteString("---\n\n")
	sb.WriteString(skill.Body)
	sb.WriteString("\n")

	content := []byte(sb.String())
	if err := os.WriteFile(target, content, 0644); err != nil {
		return "", fmt.Errorf("skills: install write: %w", err)
	}
	return target, nil
}
