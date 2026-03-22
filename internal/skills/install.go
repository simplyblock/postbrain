package skills

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/simplyblock/postbrain/internal/db"
)

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
	target := TargetPath(skill.Slug, agentType, workdir)
	if err := os.MkdirAll(filepath.Dir(target), 0755); err != nil {
		return "", fmt.Errorf("skills: install mkdir: %w", err)
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
