package codegraph

import (
	"context"
	"fmt"
	"log/slog"
	"net"
	"os"
	"path/filepath"
	"strings"

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

func lspClientForIndex(ctx context.Context, opts IndexOptions) lsp.Client {
	if opts.GoLSPRootDir == "" {
		return nil
	}
	client, err := newLSPClientForExt(".go", opts.GoLSPRootDir, opts.GoLSPTimeout)
	if err != nil {
		slog.WarnContext(ctx, "codegraph: gopls client unavailable; continuing without lsp",
			"root", opts.GoLSPRootDir, "err", err)
		return nil
	}
	return client
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
