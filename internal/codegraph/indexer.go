package codegraph

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"strings"

	"net"

	"github.com/go-git/go-billy/v5/memfs"
	gogit "github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing"
	"github.com/go-git/go-git/v5/plumbing/object"
	"github.com/go-git/go-git/v5/plumbing/transport"
	"github.com/go-git/go-git/v5/plumbing/transport/http"
	gitssh "github.com/go-git/go-git/v5/plumbing/transport/ssh"
	"github.com/go-git/go-git/v5/storage/memory"
	merkletrie "github.com/go-git/go-git/v5/utils/merkletrie"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"golang.org/x/crypto/ssh"
	"golang.org/x/crypto/ssh/agent"

	"github.com/simplyblock/postbrain/internal/db"
)

// IndexOptions controls how a repository is indexed.
type IndexOptions struct {
	// ScopeID is the project scope to associate symbols and relations with.
	ScopeID uuid.UUID
	// RepoURL is the git clone URL.
	RepoURL string
	// DefaultBranch is the branch to check out (defaults to "main").
	DefaultBranch string
	// AuthorID is the principal UUID recorded as the author of created memories.
	// If zero, memories are not created (code graph entities/relations are still indexed).
	AuthorID uuid.UUID
	// AuthToken is an optional bearer token injected as HTTP basic auth password.
	AuthToken string
	// SSHKey is an optional PEM-encoded private key used for SSH clone URLs
	// (e.g. git@github.com:user/repo.git or ssh://git@...).
	// SSHKeyPassphrase is the passphrase for the key, if encrypted.
	SSHKey           string
	SSHKeyPassphrase string
	// PrevCommit is the last indexed commit SHA; if non-empty, performs an
	// incremental diff rather than a full re-index.
	PrevCommit string
	// MaxBytesPerFile caps the maximum file size that will be parsed.
	// Files larger than this are silently skipped. 0 → 512 KiB.
	MaxBytesPerFile int64
}

const defaultMaxBytes int64 = 512 * 1024

// IndexResult summarises what was written during an index run.
type IndexResult struct {
	CommitSHA         string
	FilesIndexed      int
	FilesSkipped      int
	SymbolsUpserted   int
	RelationsUpserted int
	ChunksCreated     int
}

// IndexRepo clones (or diffs) the repository in-memory and upserts all extracted
// symbols and relations into the database. Returns the HEAD commit SHA on success.
func IndexRepo(ctx context.Context, pool *pgxpool.Pool, opts IndexOptions) (*IndexResult, error) {
	if opts.MaxBytesPerFile <= 0 {
		opts.MaxBytesPerFile = defaultMaxBytes
	}
	if opts.DefaultBranch == "" {
		opts.DefaultBranch = "main"
	}

	cloneOpts := &gogit.CloneOptions{
		URL:           opts.RepoURL,
		SingleBranch:  true,
		Tags:          gogit.NoTags,
		Depth:         1,
		ReferenceName: plumbing.NewBranchReferenceName(opts.DefaultBranch),
	}
	if isSSHURL(opts.RepoURL) {
		auth, err := sshAuth(opts.RepoURL, opts.SSHKey, opts.SSHKeyPassphrase)
		if err != nil {
			return nil, fmt.Errorf("codegraph: ssh auth: %w", err)
		}
		cloneOpts.Auth = auth
	} else if opts.AuthToken != "" {
		cloneOpts.Auth = &http.BasicAuth{
			Username: "token",
			Password: opts.AuthToken,
		}
	}

	slog.InfoContext(ctx, "codegraph: cloning repository",
		"url", sanitizeURL(opts.RepoURL),
		"branch", opts.DefaultBranch,
		"scope_id", opts.ScopeID,
	)

	repo, err := gogit.CloneContext(ctx, memory.NewStorage(), memfs.New(), cloneOpts)
	if err != nil {
		return nil, fmt.Errorf("codegraph: clone: %w", err)
	}

	head, err := repo.Head()
	if err != nil {
		return nil, fmt.Errorf("codegraph: head: %w", err)
	}
	headSHA := head.Hash().String()

	headCommit, err := repo.CommitObject(head.Hash())
	if err != nil {
		return nil, fmt.Errorf("codegraph: head commit: %w", err)
	}
	headTree, err := headCommit.Tree()
	if err != nil {
		return nil, fmt.Errorf("codegraph: head tree: %w", err)
	}

	res := &IndexResult{CommitSHA: headSHA}

	if opts.PrevCommit != "" && opts.PrevCommit != headSHA {
		prevHash := plumbing.NewHash(opts.PrevCommit)
		prevCommit, fetchErr := repo.CommitObject(prevHash)
		if fetchErr != nil {
			// prev commit unreachable in shallow clone — full re-index
			slog.WarnContext(ctx, "codegraph: prev commit not reachable, falling back to full index",
				"prev_sha", opts.PrevCommit)
			if err2 := indexFullTree(ctx, pool, opts, headTree, res); err2 != nil {
				return nil, err2
			}
		} else {
			prevTree, treeErr := prevCommit.Tree()
			if treeErr != nil {
				return nil, fmt.Errorf("codegraph: prev tree: %w", treeErr)
			}
			if err2 := indexDiff(ctx, pool, opts, prevTree, headTree, res); err2 != nil {
				return nil, err2
			}
		}
	} else {
		if err2 := indexFullTree(ctx, pool, opts, headTree, res); err2 != nil {
			return nil, err2
		}
	}

	return res, nil
}

// indexFullTree walks every file in the tree and upserts symbols/relations.
func indexFullTree(ctx context.Context, pool *pgxpool.Pool, opts IndexOptions, tree *object.Tree, res *IndexResult) error {
	return tree.Files().ForEach(func(f *object.File) error {
		if err := indexFile(ctx, pool, opts, f, res); err != nil {
			slog.WarnContext(ctx, "codegraph: index file error", "file", f.Name, "err", err)
		}
		return nil
	})
}

// indexDiff re-extracts only added/modified files and deletes relations for removed files.
func indexDiff(ctx context.Context, pool *pgxpool.Pool, opts IndexOptions, prevTree, currTree *object.Tree, res *IndexResult) error {
	changes, err := prevTree.Diff(currTree)
	if err != nil {
		return fmt.Errorf("codegraph: tree diff: %w", err)
	}

	for _, change := range changes {
		action, err := change.Action()
		if err != nil {
			continue
		}
		switch action {
		case merkletrie.Delete:
			_ = db.DeleteRelationsBySourceFile(ctx, pool, opts.ScopeID, change.From.Name)

		case merkletrie.Insert:
			f, err := currTree.File(change.To.Name)
			if err != nil {
				continue
			}
			if err := indexFile(ctx, pool, opts, f, res); err != nil {
				slog.WarnContext(ctx, "codegraph: index file error (insert)", "file", f.Name, "err", err)
			}

		case merkletrie.Modify:
			_ = db.DeleteRelationsBySourceFile(ctx, pool, opts.ScopeID, change.To.Name)
			f, err := currTree.File(change.To.Name)
			if err != nil {
				continue
			}
			if err := indexFile(ctx, pool, opts, f, res); err != nil {
				slog.WarnContext(ctx, "codegraph: index file error (modify)", "file", f.Name, "err", err)
			}
		}
	}
	return nil
}

// indexFile extracts symbols and relations from a single git blob and upserts them.
func indexFile(ctx context.Context, pool *pgxpool.Pool, opts IndexOptions, f *object.File, res *IndexResult) error {
	if f.Size > opts.MaxBytesPerFile {
		res.FilesSkipped++
		return nil
	}

	rc, err := f.Reader()
	if err != nil {
		res.FilesSkipped++
		return nil
	}
	defer rc.Close()

	src, err := io.ReadAll(rc)
	if err != nil {
		res.FilesSkipped++
		return nil
	}

	syms, edges, err := Extract(ctx, src, f.Name)
	if err != nil {
		if isUnsupported(err) {
			res.FilesSkipped++
			return nil
		}
		return err
	}

	res.FilesIndexed++
	sourceFile := f.Name

	// Create a file-level memory for the source content (only when a valid author is known).
	var fileMemoryID *uuid.UUID
	if opts.AuthorID != uuid.Nil {
		fileSourceRef := "file:" + f.Name
		fileMem, memErr := db.CreateMemory(ctx, pool, &db.Memory{
			MemoryType:      "semantic",
			ScopeID:         opts.ScopeID,
			AuthorID:        opts.AuthorID,
			Content:         string(src),
			ContentKind:     "code",
			SourceRef:       &fileSourceRef,
			PromotionStatus: "none",
		})
		if memErr != nil {
			slog.WarnContext(ctx, "codegraph: create file memory", "file", f.Name, "err", memErr)
		} else if fileMem != nil {
			fileMemoryID = &fileMem.ID
		}
	}

	// Upsert symbols as entities.
	resolver := NewResolver(pool, opts.ScopeID, nil)
	symToID := make(map[string]uuid.UUID, len(syms))
	for _, sym := range syms {
		canonical := sym.Name
		if sym.Package != "" {
			canonical = sym.Package + "." + sym.Name
		}
		ent, uErr := db.UpsertEntity(ctx, pool, &db.Entity{
			ScopeID:    opts.ScopeID,
			EntityType: string(sym.Kind),
			Name:       sym.Name,
			Canonical:  canonical,
		})
		if uErr != nil {
			slog.WarnContext(ctx, "codegraph: upsert entity", "name", sym.Name, "err", uErr)
			continue
		}
		symToID[sym.Name] = ent.ID
		symToID[canonical] = ent.ID

		// Link file memory to the file entity (KindFile symbols).
		if sym.Kind == KindFile && fileMemoryID != nil {
			if lErr := db.LinkMemoryToEntity(ctx, pool, *fileMemoryID, ent.ID, ""); lErr != nil {
				slog.WarnContext(ctx, "codegraph: link file memory to entity", "err", lErr)
			}
		}

		res.SymbolsUpserted++
	}

	// Create chunk memories for substantive code symbols.
	if fileMemoryID != nil {
		for _, sym := range syms {
			switch sym.Kind {
			case KindFunction, KindMethod, KindClass, KindStruct, KindInterface:
				if sym.StartByte == sym.EndByte {
					continue
				}
				if int(sym.EndByte) > len(src) || int(sym.StartByte) >= len(src) {
					continue
				}
				chunkContent := string(src[sym.StartByte:sym.EndByte])
				chunkSourceRef := fmt.Sprintf("file:%s:%d", f.Name, sym.StartLine+1)
				_, cErr := db.CreateMemory(ctx, pool, &db.Memory{
					MemoryType:      "semantic",
					ScopeID:         opts.ScopeID,
					AuthorID:        opts.AuthorID,
					Content:         chunkContent,
					ContentKind:     "code",
					SourceRef:       &chunkSourceRef,
					ParentMemoryID:  fileMemoryID,
					PromotionStatus: "none",
				})
				if cErr != nil {
					slog.WarnContext(ctx, "codegraph: create chunk memory", "sym", sym.Name, "err", cErr)
					continue
				}
				res.ChunksCreated++
			}
		}
	}

	// Upsert relations.
	for _, edge := range edges {
		subjectID, ok := resolver.Resolve(ctx, f.Name, edge.SubjectName, symToID)
		if !ok {
			continue
		}
		objectID, ok := resolver.Resolve(ctx, f.Name, edge.ObjectName, symToID)
		if !ok {
			continue
		}

		_, rErr := db.UpsertRelation(ctx, pool, &db.Relation{
			ScopeID:    opts.ScopeID,
			SubjectID:  subjectID,
			Predicate:  edge.Predicate,
			ObjectID:   objectID,
			Confidence: 1.0,
			SourceFile: &sourceFile,
		})
		if rErr != nil {
			slog.WarnContext(ctx, "codegraph: upsert relation",
				"predicate", edge.Predicate, "err", rErr)
			continue
		}
		res.RelationsUpserted++
	}

	return nil
}

// isSSHURL reports whether u is an SSH clone URL (git@ SCP syntax or ssh:// scheme).
func isSSHURL(u string) bool {
	return strings.HasPrefix(u, "ssh://") || strings.Contains(u, "@") && !strings.HasPrefix(u, "http")
}

// sshUserFromURL extracts the username from an SSH URL.
// For "git@github.com:user/repo.git" it returns "git".
// Falls back to "git" for any unrecognised form.
func sshUserFromURL(u string) string {
	// ssh://user@host/...
	if strings.HasPrefix(u, "ssh://") {
		rest := strings.TrimPrefix(u, "ssh://")
		if at := strings.Index(rest, "@"); at != -1 {
			return rest[:at]
		}
		return "git"
	}
	// user@host:path
	if at := strings.Index(u, "@"); at != -1 {
		return u[:at]
	}
	return "git"
}

// sshAuth resolves the go-git SSH auth method to use for a clone.
//
// With an explicit key the caller is in full control. Without one, all
// available signers are collected (SSH agent + default key files) and
// presented together so the remote can pick whichever it accepts — the
// same behaviour as the native ssh(1) client.
func sshAuth(repoURL, sshKey, passphrase string) (transport.AuthMethod, error) {
	user := sshUserFromURL(repoURL)
	hostKeyHelper := gitssh.HostKeyCallbackHelper{
		HostKeyCallback: ssh.InsecureIgnoreHostKey(), //nolint:gosec // private repos on known hosts
	}

	// 1. Explicit PEM key provided by the caller.
	if sshKey != "" {
		signer, err := parseSSHKey([]byte(sshKey), passphrase)
		if err != nil {
			return nil, err
		}
		return &gitssh.PublicKeys{User: user, Signer: signer, HostKeyCallbackHelper: hostKeyHelper}, nil
	}

	// 2. Collect all available signers so the remote can pick any matching key.
	var signers []ssh.Signer

	// From SSH agent.
	if sock := os.Getenv("SSH_AUTH_SOCK"); sock != "" {
		if conn, err := net.Dial("unix", sock); err == nil {
			if s, err := agent.NewClient(conn).Signers(); err == nil {
				signers = append(signers, s...)
			}
			conn.Close()
		}
	}

	// From default key files (skipped if unreadable or passphrase-protected without a passphrase).
	if home, err := os.UserHomeDir(); err == nil {
		for _, name := range []string{"id_ed25519", "id_ecdsa", "id_rsa"} {
			pem, err := os.ReadFile(filepath.Join(home, ".ssh", name))
			if err != nil {
				continue
			}
			signer, err := parseSSHKey(pem, passphrase)
			if err != nil {
				continue
			}
			signers = append(signers, signer)
		}
	}

	if len(signers) == 0 {
		return nil, fmt.Errorf("no SSH credentials available: provide an ssh_key, run an SSH agent, or place a key in ~/.ssh")
	}

	snapshot := signers // capture for closure
	return &gitssh.PublicKeysCallback{
		User: user,
		Callback: func() ([]ssh.Signer, error) {
			return snapshot, nil
		},
		HostKeyCallbackHelper: hostKeyHelper,
	}, nil
}

// parseSSHKey parses a PEM-encoded private key, decrypting it with passphrase if non-empty.
func parseSSHKey(pemBytes []byte, passphrase string) (ssh.Signer, error) {
	if passphrase != "" {
		return ssh.ParsePrivateKeyWithPassphrase(pemBytes, []byte(passphrase))
	}
	return ssh.ParsePrivateKey(pemBytes)
}

// sanitizeURL removes credentials from a URL for safe logging.
func sanitizeURL(u string) string {
	if idx := strings.Index(u, "@"); idx != -1 {
		if schemeEnd := strings.Index(u, "://"); schemeEnd != -1 {
			return u[:schemeEnd+3] + u[idx+1:]
		}
	}
	return u
}

func isUnsupported(err error) bool {
	_, ok := err.(ErrUnsupportedLanguage)
	return ok
}
