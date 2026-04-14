package codegraph

import (
	"context"
	"fmt"
	"log/slog"
	"net"
	"os"
	"path/filepath"
	"strings"
	"time"

	gogit "github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing"
	"github.com/go-git/go-git/v5/plumbing/transport"
	"github.com/go-git/go-git/v5/plumbing/transport/http"
	gitssh "github.com/go-git/go-git/v5/plumbing/transport/ssh"
	"golang.org/x/crypto/ssh"
	"golang.org/x/crypto/ssh/agent"

	"github.com/simplyblock/postbrain/internal/closeutil"
	"github.com/simplyblock/postbrain/internal/codegraph/lsp"
)

// newLSPClientForExt is the factory used to create an lsp.Client for a given
// file extension.  It is a package-level variable so tests can inject fakes.
var newLSPClientForExt = lsp.NewClientForExt

type lspSelection struct {
	canonicalExt string
	rootDir      string
	timeout      time.Duration
	clientOpts   lsp.ClientOptions
	hintExts     map[string]int

	initialized bool
	client      lsp.Client
}

// buildCloneAuth returns the go-git auth method for opts, or nil if no auth is configured.
func buildCloneAuth(opts IndexOptions) (transport.AuthMethod, error) {
	if isSSHURL(opts.RepoURL) {
		return sshAuth(opts.RepoURL, opts.SSHKey, opts.SSHKeyPassphrase)
	}
	if opts.AuthToken != "" {
		return &http.BasicAuth{Username: "token", Password: opts.AuthToken}, nil
	}
	return nil, nil
}

func newLSPRegistry(opts IndexOptions) []lspSelection {
	var out []lspSelection

	if opts.GoLSPRootDir != "" {
		out = append(out, lspSelection{
			canonicalExt: ".go",
			rootDir:      opts.GoLSPRootDir,
			timeout:      opts.GoLSPTimeout,
			hintExts: map[string]int{
				".go": 100,
			},
		})
	}

	if opts.TypeScriptLSPRootDir != "" {
		out = append(out, lspSelection{
			canonicalExt: ".ts",
			rootDir:      opts.TypeScriptLSPRootDir,
			timeout:      opts.TypeScriptLSPTimeout,
			clientOpts: lsp.ClientOptions{
				UseTSGo: opts.TypeScriptLSPUseTSGo,
			},
			hintExts: map[string]int{
				".ts":  100,
				".tsx": 95,
				".js":  90,
				".jsx": 85,
			},
		})
	}

	if opts.ClangdLSPRootDir != "" {
		out = append(out, lspSelection{
			canonicalExt: ".c",
			rootDir:      opts.ClangdLSPRootDir,
			timeout:      opts.ClangdLSPTimeout,
			hintExts: map[string]int{
				".c":   100,
				".h":   95,
				".hpp": 95,
				".hh":  95,
				".cpp": 90,
				".cc":  90,
				".cxx": 90,
			},
		})
	}

	if opts.PythonLSPRootDir != "" {
		out = append(out, lspSelection{
			canonicalExt: ".py",
			rootDir:      opts.PythonLSPRootDir,
			timeout:      opts.PythonLSPTimeout,
			hintExts: map[string]int{
				".py": 100,
			},
		})
	}

	if opts.MarkdownLSPRootDir != "" {
		out = append(out, lspSelection{
			canonicalExt: ".md",
			rootDir:      opts.MarkdownLSPRootDir,
			timeout:      opts.MarkdownLSPTimeout,
			hintExts: map[string]int{
				".md":       100,
				".markdown": 95,
			},
		})
	}

	return out
}

func lspClientForFile(ctx context.Context, filePath string, selections []lspSelection) (lsp.Client, string) {
	ext := strings.ToLower(filepath.Ext(filePath))
	if ext == "" || len(selections) == 0 {
		return nil, ""
	}
	var (
		chosenClient lsp.Client
		chosenRoot   string
		bestPriority int
		haveBest     bool
	)
	for i := range selections {
		sel := &selections[i]
		_, hinted := sel.hintExts[ext]
		if !hinted && sel.client == nil {
			continue
		}
		client := ensureLSPSelectionClient(ctx, sel)
		if client == nil {
			continue
		}
		priority, ok := client.SupportedLanguages()[ext]
		if !ok {
			if hinted {
				priority = sel.hintExts[ext]
			} else {
				continue
			}
		}
		if !haveBest || priority > bestPriority {
			haveBest = true
			bestPriority = priority
			chosenClient = client
			chosenRoot = sel.rootDir
		}
	}
	if !haveBest {
		return nil, ""
	}
	return chosenClient, chosenRoot
}

func ensureLSPSelectionClient(ctx context.Context, sel *lspSelection) lsp.Client {
	if sel == nil {
		return nil
	}
	if sel.client != nil {
		sel.initialized = true
		return sel.client
	}
	if sel.initialized {
		return sel.client
	}
	sel.initialized = true
	client, err := newLSPClientForExt(sel.canonicalExt, sel.rootDir, sel.timeout, sel.clientOpts)
	if err != nil {
		slog.WarnContext(ctx, "codegraph: lsp client unavailable; continuing without lsp",
			"ext", sel.canonicalExt, "root", sel.rootDir, "err", err)
		return nil
	}
	sel.client = client
	return sel.client
}

func closeLSPSelections(ctx context.Context, selections []lspSelection) {
	for _, sel := range selections {
		if sel.client == nil {
			continue
		}
		if err := sel.client.Close(); err != nil {
			slog.WarnContext(ctx, "codegraph: close lsp client", "err", err)
		}
	}
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
			closeutil.Log(conn, "ssh agent socket")
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

// newInMemoryCloneOptions builds CloneOptions for an in-memory shallow clone.
func newInMemoryCloneOptions(opts IndexOptions) (*gogit.CloneOptions, error) {
	depth := opts.Depth
	if depth <= 0 {
		depth = 1
	}
	cloneOpts := &gogit.CloneOptions{
		URL:           opts.RepoURL,
		SingleBranch:  true,
		Tags:          gogit.NoTags,
		Depth:         depth,
		ReferenceName: plumbing.NewBranchReferenceName(opts.DefaultBranch),
	}
	auth, err := buildCloneAuth(opts)
	if err != nil {
		return nil, err
	}
	cloneOpts.Auth = auth
	return cloneOpts, nil
}
