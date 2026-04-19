package skills

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/simplyblock/postbrain/internal/db"
	"github.com/simplyblock/postbrain/internal/db/compat"
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

// TargetPath returns the absolute path of the main SKILL.md for a skill.
//   - codex agent: {workdir}/.agents/skills/{slug}/SKILL.md
//   - all others:  {workdir}/.claude/skills/{slug}/SKILL.md
func TargetPath(slug, agentType, workdir string) string {
	if agentType == "codex" {
		return filepath.Join(workdir, ".agents", "skills", slug, "SKILL.md")
	}
	return filepath.Join(workdir, ".claude", "skills", slug, "SKILL.md")
}

// SkillDirPath returns the directory that contains the skill's SKILL.md and
// any supplementary files (scripts/, references/).
func SkillDirPath(slug, agentType, workdir string) string {
	return filepath.Dir(TargetPath(slug, agentType, workdir))
}

// IsInstalled reports whether the skill's SKILL.md exists at the expected path.
func IsInstalled(slug, agentType, workdir string) bool {
	_, err := os.Stat(TargetPath(slug, agentType, workdir))
	return err == nil
}

// Install materialises a skill to the agent's skills directory and returns the
// absolute path of the written SKILL.md. Supplementary files are written into
// typed subdirectories within the skill directory:
//   - scripts/    — executable files (is_executable=true)
//   - references/ — additional markdown files (.md extension)
//
// For claude-code agents, an @-reference is injected into .claude/CLAUDE.md
// so that Claude Code automatically loads the skill as agent context.
// Passing nil or empty files installs only SKILL.md (backward-compatible).
func Install(skill *db.Skill, files []*db.SkillFile, agentType, workdir string) (string, error) {
	if err := ValidateSlug(skill.Slug); err != nil {
		return "", err
	}

	target := TargetPath(skill.Slug, agentType, workdir)
	skillDir := filepath.Dir(target)

	// Create the skill directory so EvalSymlinks can resolve it below.
	if err := os.MkdirAll(skillDir, 0755); err != nil {
		return "", fmt.Errorf("skills: install mkdir: %w", err)
	}

	// Defense-in-depth: resolve symlinks on both paths before comparing so that
	// a symlink under workdir (e.g. workdir/.claude → /etc) cannot be used to
	// escape the intended base directory. filepath.Abs alone does not protect
	// against this — filepath.EvalSymlinks follows every symlink component.
	realBase, err := filepath.EvalSymlinks(workdir)
	if err != nil {
		return "", fmt.Errorf("skills: install resolve base: %w", err)
	}
	// The target file itself does not exist yet; resolve its parent (which was
	// just created by MkdirAll) and re-attach the filename.
	realDir, err := filepath.EvalSymlinks(skillDir)
	if err != nil {
		return "", fmt.Errorf("skills: install resolve target dir: %w", err)
	}
	realTarget := filepath.Join(realDir, filepath.Base(target))
	if !strings.HasPrefix(realTarget, filepath.Clean(realBase)+string(filepath.Separator)) {
		return "", fmt.Errorf("skills: install path %q escapes base directory %q", realTarget, realBase)
	}

	// Build and write SKILL.md.
	content, err := buildSkillMD(skill)
	if err != nil {
		return "", err
	}
	if err := os.WriteFile(realTarget, content, 0644); err != nil {
		return "", fmt.Errorf("skills: install write: %w", err)
	}

	// Write supplementary files.
	for _, f := range files {
		if err := installFile(f, realBase, realDir); err != nil {
			return "", err
		}
	}

	// Inject @-reference into .claude/CLAUDE.md for claude-code agents.
	if agentType != "codex" {
		if err := injectClaudeMDReference(skill.Slug, workdir); err != nil {
			return "", fmt.Errorf("skills: install inject CLAUDE.md: %w", err)
		}
	}

	return realTarget, nil
}

// InstallFromDB fetches supplementary files from the database and calls Install.
// Returns the main SKILL.md path, a list of supplementary file paths, and any error.
func InstallFromDB(ctx context.Context, pool *pgxpool.Pool, skill *db.Skill, agentType, workdir string) (string, []string, error) {
	files, err := compat.ListSkillFiles(ctx, pool, skill.ID)
	if err != nil {
		return "", nil, fmt.Errorf("skills: install list files: %w", err)
	}
	path, err := Install(skill, files, agentType, workdir)
	if err != nil {
		return "", nil, err
	}
	// Derive the skill directory from the resolved path returned by Install
	// rather than from SkillDirPath(workdir), which uses the unresolved workdir.
	// If workdir contains symlinks the two differ; using filepath.Dir(path)
	// guarantees the supplementary file paths agree with where Install wrote them.
	skillDir := filepath.Dir(path)
	var filePaths []string
	for _, f := range files {
		filePaths = append(filePaths, filepath.Join(skillDir, filepath.FromSlash(f.RelativePath)))
	}
	return path, filePaths, nil
}

// installFile writes one supplementary skill file to disk, applying the same
// symlink-escape defence used for SKILL.md.
func installFile(f *db.SkillFile, realBase, skillRealDir string) error {
	if err := ValidateSkillFile(db.SkillFileInput{
		RelativePath: f.RelativePath,
		Content:      f.Content,
		IsExecutable: f.IsExecutable,
	}); err != nil {
		return err
	}

	// filepath.FromSlash converts forward slashes (canonical in DB) to OS separator.
	fullPath := filepath.Join(skillRealDir, filepath.FromSlash(f.RelativePath))

	// Containment check: the file must remain inside the skill directory.
	if !strings.HasPrefix(fullPath, filepath.Clean(skillRealDir)+string(filepath.Separator)) {
		return fmt.Errorf("skills: supplementary file %q escapes skill directory", f.RelativePath)
	}
	// Also ensure it stays within the overall workdir.
	if !strings.HasPrefix(fullPath, filepath.Clean(realBase)+string(filepath.Separator)) {
		return fmt.Errorf("skills: supplementary file %q escapes base directory", f.RelativePath)
	}

	parentDir := filepath.Dir(fullPath)
	if err := os.MkdirAll(parentDir, 0755); err != nil {
		return fmt.Errorf("skills: install mkdir for %q: %w", f.RelativePath, err)
	}

	// Resolve symlinks on the parent directory now that it exists.
	// A pre-existing symlink (e.g. scripts/ → /etc) would pass the string
	// prefix check above but WriteFile would follow it outside the skill dir.
	// EvalSymlinks on the parent catches that before any write happens.
	realParentDir, err := filepath.EvalSymlinks(parentDir)
	if err != nil {
		return fmt.Errorf("skills: install resolve parent for %q: %w", f.RelativePath, err)
	}
	if !strings.HasPrefix(realParentDir, filepath.Clean(skillRealDir)+string(filepath.Separator)) {
		return fmt.Errorf("skills: supplementary file %q parent escapes skill directory after symlink resolution", f.RelativePath)
	}
	if !strings.HasPrefix(realParentDir, filepath.Clean(realBase)+string(filepath.Separator)) {
		return fmt.Errorf("skills: supplementary file %q parent escapes base directory after symlink resolution", f.RelativePath)
	}

	// Write to the resolved path so the OS call itself never follows a symlink.
	realFullPath := filepath.Join(realParentDir, filepath.Base(fullPath))
	mode := os.FileMode(0644)
	if f.IsExecutable {
		mode = 0755
	}
	if err := os.WriteFile(realFullPath, []byte(f.Content), mode); err != nil {
		return fmt.Errorf("skills: install write %q: %w", f.RelativePath, err)
	}
	return nil
}

// injectClaudeMDReference appends an @-reference for the given skill into
// .claude/CLAUDE.md, creating the file if it does not exist. The operation
// is idempotent: if the reference is already present the file is unchanged.
func injectClaudeMDReference(slug, workdir string) error {
	dir := filepath.Join(workdir, ".claude")
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("mkdir .claude: %w", err)
	}

	claudeMD := filepath.Join(dir, "CLAUDE.md")
	ref := fmt.Sprintf("@skills/%s/SKILL.md", slug)

	existing, err := os.ReadFile(claudeMD)
	if err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("read .claude/CLAUDE.md: %w", err)
	}

	if strings.Contains(string(existing), ref) {
		return nil // already present — idempotent
	}

	var sb strings.Builder
	sb.Write(existing)
	if len(existing) > 0 && !strings.HasSuffix(string(existing), "\n") {
		sb.WriteByte('\n')
	}
	sb.WriteString(ref)
	sb.WriteByte('\n')

	return os.WriteFile(claudeMD, []byte(sb.String()), 0644)
}

// buildSkillMD assembles the YAML frontmatter + body content for SKILL.md.
func buildSkillMD(skill *db.Skill) ([]byte, error) {
	agentTypesJSON, err := json.Marshal(skill.AgentTypes)
	if err != nil {
		return nil, fmt.Errorf("skills: install marshal agent_types: %w", err)
	}

	var sb strings.Builder
	sb.WriteString("---\n")
	sb.WriteString(fmt.Sprintf("name: %s\n", skill.Name))
	sb.WriteString(fmt.Sprintf("description: %s\n", skill.Description))
	sb.WriteString(fmt.Sprintf("agent_types: %s\n", string(agentTypesJSON)))
	sb.WriteString(fmt.Sprintf("version: %d\n", skill.Version))

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

	return []byte(sb.String()), nil
}
