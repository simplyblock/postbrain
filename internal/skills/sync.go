package skills

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/simplyblock/postbrain/internal/closeutil"
	"github.com/simplyblock/postbrain/internal/db"
	"github.com/simplyblock/postbrain/internal/db/compat"
)

// SyncResult summarises the outcome of a Sync operation.
type SyncResult struct {
	Installed []string // slugs of newly installed skills
	Updated   []string // slugs of skills that were reinstalled due to version change
	Orphaned  []string // local skills not in the published registry
}

// syncDB abstracts the database queries used during sync.
type syncDB interface {
	listPublishedSkillsForAgent(ctx context.Context, scopeIDs []uuid.UUID, agentType string) ([]*db.Skill, error)
	listSkillFiles(ctx context.Context, skillID uuid.UUID) ([]*db.SkillFile, error)
}

// poolSyncDB wraps pgxpool.Pool to implement syncDB.
type poolSyncDB struct {
	pool *pgxpool.Pool
}

func (p *poolSyncDB) listPublishedSkillsForAgent(ctx context.Context, scopeIDs []uuid.UUID, agentType string) ([]*db.Skill, error) {
	return compat.ListPublishedSkillsForAgent(ctx, p.pool, scopeIDs, agentType)
}

func (p *poolSyncDB) listSkillFiles(ctx context.Context, skillID uuid.UUID) ([]*db.SkillFile, error) {
	return compat.ListSkillFiles(ctx, p.pool, skillID)
}

// Sync compares the local agent skills directory against the published skill registry
// and installs or updates missing or outdated skills.
func Sync(ctx context.Context, pool *pgxpool.Pool, scopeIDs []uuid.UUID, agentType, workdir string) (*SyncResult, error) {
	return syncInternal(ctx, &poolSyncDB{pool: pool}, scopeIDs, agentType, workdir)
}

// syncInternal is the testable core of Sync.
func syncInternal(ctx context.Context, sdb syncDB, scopeIDs []uuid.UUID, agentType, workdir string) (*SyncResult, error) {
	published, err := sdb.listPublishedSkillsForAgent(ctx, scopeIDs, agentType)
	if err != nil {
		return nil, fmt.Errorf("skills: sync list: %w", err)
	}

	// Index published skills by slug for fast lookup.
	bySlug := make(map[string]*db.Skill, len(published))
	for _, s := range published {
		bySlug[s.Slug] = s
	}

	result := &SyncResult{}

	for _, skill := range published {
		files, err := sdb.listSkillFiles(ctx, skill.ID)
		if err != nil {
			return nil, fmt.Errorf("skills: sync list files for %s: %w", skill.Slug, err)
		}

		if !IsInstalled(skill.Slug, agentType, workdir) {
			if _, err := Install(skill, files, agentType, workdir); err != nil {
				return nil, fmt.Errorf("skills: sync install %s: %w", skill.Slug, err)
			}
			result.Installed = append(result.Installed, skill.Slug)
			continue
		}

		// Check if the installed version differs from the registry version.
		installedVersion := readInstalledVersion(skill.Slug, agentType, workdir)
		if installedVersion != int(skill.Version) {
			if _, err := Install(skill, files, agentType, workdir); err != nil {
				return nil, fmt.Errorf("skills: sync update %s: %w", skill.Slug, err)
			}
			result.Updated = append(result.Updated, skill.Slug)
		}
	}

	// Find orphaned local skills.
	localSlugs, err := listLocalSkillSlugs(agentType, workdir)
	if err != nil {
		// If the directory doesn't exist there are no local skills.
		return result, nil
	}
	for _, slug := range localSlugs {
		if _, ok := bySlug[slug]; !ok {
			result.Orphaned = append(result.Orphaned, slug)
		}
	}

	return result, nil
}

// ReadInstalledVersion reads the version field from the frontmatter of an
// installed skill's SKILL.md. Returns 0 if the file cannot be read or has no
// version line (treated as outdated). Exported for use by the hook CLI.
func ReadInstalledVersion(slug, agentType, workdir string) int {
	return readInstalledVersion(slug, agentType, workdir)
}

func readInstalledVersion(slug, agentType, workdir string) int {
	// Reject any slug that would fail slug validation — defense-in-depth against
	// traversal slugs stored in the DB before the ValidateSlug check was added.
	if ValidateSlug(slug) != nil {
		return 0
	}

	// Containment check: resolve symlinks on both base and target so that a
	// symlink component under workdir cannot redirect the read outside the
	// intended base directory.
	path := TargetPath(slug, agentType, workdir)
	realBase, err := filepath.EvalSymlinks(workdir)
	if err != nil {
		return 0
	}
	// The target file may not exist; resolve its parent directory, then re-attach
	// the filename. If the parent doesn't exist either, os.Open will fail safely.
	realDir, err := filepath.EvalSymlinks(filepath.Dir(path))
	if err != nil {
		return 0
	}
	realPath := filepath.Join(realDir, filepath.Base(path))
	if !strings.HasPrefix(realPath, filepath.Clean(realBase)+string(filepath.Separator)) {
		return 0
	}

	f, err := os.Open(realPath)
	if err != nil {
		return 0
	}
	defer closeutil.Log(f, "installed skill file")

	scanner := bufio.NewScanner(f)
	inFrontmatter := false
	firstLine := true
	for scanner.Scan() {
		line := scanner.Text()
		if firstLine {
			if line == "---" {
				inFrontmatter = true
			}
			firstLine = false
			continue
		}
		if inFrontmatter {
			if line == "---" {
				break
			}
			if strings.HasPrefix(line, "version:") {
				parts := strings.SplitN(line, ":", 2)
				if len(parts) == 2 {
					v, err := strconv.Atoi(strings.TrimSpace(parts[1]))
					if err == nil {
						return v
					}
				}
			}
		}
	}
	return 0
}

// listLocalSkillSlugs returns the slugs of all locally installed skills by
// scanning the agent's skills directory for subdirectories that contain a SKILL.md.
func listLocalSkillSlugs(agentType, workdir string) ([]string, error) {
	var dir string
	if agentType == "codex" {
		dir = filepath.Join(workdir, ".agents", "skills")
	} else {
		dir = filepath.Join(workdir, ".claude", "skills")
	}

	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}

	var slugs []string
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		// Only count directories that actually contain SKILL.md.
		skillMD := filepath.Join(dir, e.Name(), "SKILL.md")
		if _, err := os.Stat(skillMD); err == nil {
			slugs = append(slugs, e.Name())
		}
	}
	return slugs, nil
}
