package codegraph

import (
	"crypto/ed25519"
	"crypto/rand"
	"encoding/pem"
	"testing"

	"golang.org/x/crypto/ssh"
)

// ── isSSHURL ──────────────────────────────────────────────────────────────────

func TestIsSSHURL(t *testing.T) {
	t.Parallel()
	tests := []struct {
		url  string
		want bool
	}{
		// SCP syntax
		{"git@github.com:user/repo.git", true},
		{"git@gitlab.com:group/project.git", true},
		// ssh:// scheme
		{"ssh://git@github.com/user/repo.git", true},
		{"ssh://deploy@host.example.com/repo", true},
		{"git+ssh://deploy@host.example.com:/~/repo.git", true},
		// HTTPS — must not be treated as SSH
		{"https://github.com/user/repo.git", false},
		{"http://github.com/user/repo.git", false},
		// Empty / local paths
		{"", false},
		{"/home/user/repo", false},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.url, func(t *testing.T) {
			t.Parallel()
			if got := isSSHURL(tt.url); got != tt.want {
				t.Errorf("isSSHURL(%q) = %v, want %v", tt.url, got, tt.want)
			}
		})
	}
}

// ── sshUserFromURL ────────────────────────────────────────────────────────────

func TestSSHUserFromURL(t *testing.T) {
	t.Parallel()
	tests := []struct {
		url  string
		want string
	}{
		// SCP syntax — user is the part before @
		{"git@github.com:user/repo.git", "git"},
		{"deploy@host.example.com:path/repo", "deploy"},
		// ssh:// scheme — user between ssh:// and @
		{"ssh://git@github.com/user/repo.git", "git"},
		{"ssh://admin@internal.host/repo", "admin"},
		// ssh:// with no @ — falls back to "git"
		{"ssh://github.com/user/repo.git", "git"},
		// No @ at all — falls back to "git"
		{"https://github.com/user/repo.git", "git"},
		{"", "git"},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.url, func(t *testing.T) {
			t.Parallel()
			if got := sshUserFromURL(tt.url); got != tt.want {
				t.Errorf("sshUserFromURL(%q) = %q, want %q", tt.url, got, tt.want)
			}
		})
	}
}

// ── sanitizeURL ───────────────────────────────────────────────────────────────

func TestSanitizeURL(t *testing.T) {
	t.Parallel()
	tests := []struct {
		url  string
		want string
	}{
		// Credentials in HTTPS URL are stripped.
		{"https://user:password@github.com/repo.git", "https://github.com/repo.git"},
		{"https://token@github.com/repo.git", "https://github.com/repo.git"},
		// SSH SCP URL has no scheme so it is left unchanged.
		{"git@github.com:user/repo.git", "git@github.com:user/repo.git"},
		// ssh:// URL: the function strips everything before @ regardless of scheme,
		// so the username is removed (acceptable since this is used only for logging).
		{"ssh://git@github.com/user/repo.git", "ssh://github.com/user/repo.git"},
		// Plain HTTPS without credentials — unchanged.
		{"https://github.com/user/repo.git", "https://github.com/user/repo.git"},
		// Empty string — unchanged.
		{"", ""},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.url, func(t *testing.T) {
			t.Parallel()
			if got := sanitizeURL(tt.url); got != tt.want {
				t.Errorf("sanitizeURL(%q) = %q, want %q", tt.url, got, tt.want)
			}
		})
	}
}

// ── parseSSHKey ───────────────────────────────────────────────────────────────

// generateUnencryptedPEM creates a fresh ed25519 private key and returns it as
// an unencrypted OpenSSH PEM block.
func generateUnencryptedPEM(t *testing.T) []byte {
	t.Helper()
	_, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatalf("generate key: %v", err)
	}
	block, err := ssh.MarshalPrivateKey(priv, "")
	if err != nil {
		t.Fatalf("marshal private key: %v", err)
	}
	return pem.EncodeToMemory(block)
}

// generateEncryptedPEM creates a fresh ed25519 private key encrypted with passphrase.
func generateEncryptedPEM(t *testing.T, passphrase string) []byte {
	t.Helper()
	_, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatalf("generate key: %v", err)
	}
	block, err := ssh.MarshalPrivateKeyWithPassphrase(priv, "", []byte(passphrase))
	if err != nil {
		t.Fatalf("marshal encrypted private key: %v", err)
	}
	return pem.EncodeToMemory(block)
}

func TestParseSSHKey_ValidUnencryptedKey(t *testing.T) {
	t.Parallel()
	pemBytes := generateUnencryptedPEM(t)
	signer, err := parseSSHKey(pemBytes, "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if signer == nil {
		t.Fatal("expected non-nil signer")
	}
}

func TestParseSSHKey_ValidEncryptedKeyWithPassphrase(t *testing.T) {
	t.Parallel()
	const passphrase = "supersecret"
	pemBytes := generateEncryptedPEM(t, passphrase)
	signer, err := parseSSHKey(pemBytes, passphrase)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if signer == nil {
		t.Fatal("expected non-nil signer")
	}
}

func TestParseSSHKey_EncryptedKeyWrongPassphraseReturnsError(t *testing.T) {
	t.Parallel()
	pemBytes := generateEncryptedPEM(t, "correctpassphrase")
	_, err := parseSSHKey(pemBytes, "wrongpassphrase")
	if err == nil {
		t.Fatal("expected error for wrong passphrase, got nil")
	}
}

func TestParseSSHKey_EncryptedKeyNoPassphraseReturnsError(t *testing.T) {
	t.Parallel()
	pemBytes := generateEncryptedPEM(t, "somepassphrase")
	_, err := parseSSHKey(pemBytes, "")
	if err == nil {
		t.Fatal("expected error when passphrase omitted for encrypted key, got nil")
	}
}

func TestParseSSHKey_GarbageInputReturnsError(t *testing.T) {
	t.Parallel()
	_, err := parseSSHKey([]byte("this is not a PEM key"), "")
	if err == nil {
		t.Fatal("expected error for garbage input, got nil")
	}
}

func TestParseSSHKey_EmptyInputReturnsError(t *testing.T) {
	t.Parallel()
	_, err := parseSSHKey([]byte{}, "")
	if err == nil {
		t.Fatal("expected error for empty input, got nil")
	}
}
