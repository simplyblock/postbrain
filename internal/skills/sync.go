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

	"github.com/simplyblock/postbrain/internal/db"
)

// SyncResult summarises the outcome of a Sync operation.
type SyncResult struct {
	Installed []string // slugs of newly installed skills
	Updated   []string // slugs of skills that were reinstalled due to version change
	Orphaned  []string // local files not in the published registry
}

// syncDB abstracts the database query used during sync.
type syncDB interface {
	listPublishedSkillsForAgent(ctx context.Context, scopeIDs []uuid.UUID, agentType string) ([]*db.Skill, error)
}

// poolSyncDB wraps pgxpool.Pool to implement syncDB.
type poolSyncDB struct {
	pool *pgxpool.Pool
}

func (p *poolSyncDB) listPublishedSkillsForAgent(ctx context.Context, scopeIDs []uuid.UUID, agentType string) ([]*db.Skill, error) {
	return db.ListPublishedSkillsForAgent(ctx, p.pool, scopeIDs, agentType)
}

// Sync compares the local agent command directory against the published skill registry
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
		if !IsInstalled(skill.Slug, agentType, workdir) {
			if _, err := Install(skill, agentType, workdir); err != nil {
				return nil, fmt.Errorf("skills: sync install %s: %w", skill.Slug, err)
			}
			result.Installed = append(result.Installed, skill.Slug)
			continue
		}

		// Check if the installed version differs from the registry version.
		installedVersion := readInstalledVersion(skill.Slug, agentType, workdir)
		if installedVersion != int(skill.Version) {
			if _, err := Install(skill, agentType, workdir); err != nil {
				return nil, fmt.Errorf("skills: sync update %s: %w", skill.Slug, err)
			}
			result.Updated = append(result.Updated, skill.Slug)
		}
	}

	// Find orphaned local files.
	localFiles, err := listLocalSkillFiles(agentType, workdir)
	if err != nil {
		// If the directory doesn't exist there are no local files.
		return result, nil
	}
	for _, slug := range localFiles {
		if _, ok := bySlug[slug]; !ok {
			result.Orphaned = append(result.Orphaned, slug)
		}
	}

	return result, nil
}

// readInstalledVersion reads the version field from the frontmatter of an installed skill file.
// Returns 0 if the file cannot be read or has no version line (treated as outdated).
func readInstalledVersion(slug, agentType, workdir string) int {
	path := TargetPath(slug, agentType, workdir)
	f, err := os.Open(path)
	if err != nil {
		return 0
	}
	defer f.Close()

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

// listLocalSkillFiles returns the slugs (without .md suffix) of all .md files
// in the agent's command directory.
func listLocalSkillFiles(agentType, workdir string) ([]string, error) {
	var dir string
	if agentType == "codex" {
		dir = filepath.Join(workdir, ".codex", "skills")
	} else {
		dir = filepath.Join(workdir, ".claude", "commands")
	}

	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}

	var slugs []string
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		name := e.Name()
		if strings.HasSuffix(name, ".md") {
			slugs = append(slugs, strings.TrimSuffix(name, ".md"))
		}
	}
	return slugs, nil
}
