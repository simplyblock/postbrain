package codegraph

import (
	"errors"
	"fmt"
	"testing"
)

// stubResolver is a dnsResolver replacement for tests that must not touch the network.
func stubResolver(addrs []string, err error) func(string) ([]string, error) {
	return func(_ string) ([]string, error) { return addrs, err }
}

func TestValidateRepoURL(t *testing.T) {
	t.Parallel()

	// Use a stub resolver for all hostname tests so the suite runs offline.
	publicResolver := stubResolver([]string{"140.82.113.4"}, nil) // github.com-like
	privateResolver := stubResolver([]string{"192.168.1.1"}, nil)
	loopbackResolver := stubResolver([]string{"127.0.0.1"}, nil)
	unreachableResolver := stubResolver(nil, errors.New("no such host"))

	tests := []struct {
		name     string
		url      string
		resolver func(string) ([]string, error)
		wantErr  bool
	}{
		// ── valid URLs ───────────────────────────────────────────────────────────
		{name: "https github", url: "https://github.com/org/repo.git", resolver: publicResolver},
		{name: "http public host", url: "http://gitlab.example.com/org/repo.git", resolver: publicResolver},
		{name: "ssh url", url: "ssh://git@github.com/org/repo.git", resolver: publicResolver},
		{name: "git scheme", url: "git://github.com/org/repo.git", resolver: publicResolver},
		{name: "git+ssh scheme", url: "git+ssh://git@github.com/org/repo.git", resolver: publicResolver},
		{name: "https with port", url: "https://github.com:443/org/repo.git", resolver: publicResolver},
		{name: "https with credentials", url: "https://user:token@github.com/org/repo.git", resolver: publicResolver},

		// ── empty / unparseable ──────────────────────────────────────────────────
		{name: "empty string", url: "", resolver: publicResolver, wantErr: true},

		// ── disallowed schemes ───────────────────────────────────────────────────
		{name: "file scheme", url: "file:///etc/passwd", resolver: publicResolver, wantErr: true},
		{name: "ftp scheme", url: "ftp://github.com/repo.git", resolver: publicResolver, wantErr: true},
		{name: "data scheme", url: "data:text/plain,hello", resolver: publicResolver, wantErr: true},
		{name: "no scheme", url: "github.com/org/repo.git", resolver: publicResolver, wantErr: true},

		// ── missing host ─────────────────────────────────────────────────────────
		{name: "missing host", url: "https:///no-host", resolver: publicResolver, wantErr: true},

		// ── loopback hostnames ───────────────────────────────────────────────────
		{name: "localhost hostname", url: "https://localhost/repo.git", resolver: publicResolver, wantErr: true},
		{name: "LOCALHOST uppercase", url: "https://LOCALHOST/repo.git", resolver: publicResolver, wantErr: true},

		// ── literal loopback IPs ─────────────────────────────────────────────────
		{name: "loopback 127.0.0.1", url: "https://127.0.0.1/repo.git", resolver: publicResolver, wantErr: true},
		{name: "loopback 127.1.2.3", url: "https://127.1.2.3/repo.git", resolver: publicResolver, wantErr: true},
		{name: "ipv6 loopback ::1", url: "https://[::1]/repo.git", resolver: publicResolver, wantErr: true},

		// ── link-local (metadata services, APIPA) ────────────────────────────────
		{name: "aws metadata 169.254.169.254", url: "https://169.254.169.254/latest/meta-data", resolver: publicResolver, wantErr: true},
		{name: "link-local 169.254.0.1", url: "https://169.254.0.1/repo.git", resolver: publicResolver, wantErr: true},
		{name: "ipv6 link-local fe80::1", url: "https://[fe80::1]/repo.git", resolver: publicResolver, wantErr: true},

		// ── RFC-1918 private ranges ───────────────────────────────────────────────
		{name: "private 10.0.0.1", url: "https://10.0.0.1/repo.git", resolver: publicResolver, wantErr: true},
		{name: "private 10.255.255.255", url: "https://10.255.255.255/repo.git", resolver: publicResolver, wantErr: true},
		{name: "private 172.16.0.1", url: "https://172.16.0.1/repo.git", resolver: publicResolver, wantErr: true},
		{name: "private 172.31.255.255", url: "https://172.31.255.255/repo.git", resolver: publicResolver, wantErr: true},
		{name: "private 192.168.0.1", url: "https://192.168.0.1/repo.git", resolver: publicResolver, wantErr: true},
		{name: "private 192.168.255.255", url: "https://192.168.255.255/repo.git", resolver: publicResolver, wantErr: true},

		// ── other reserved ranges ────────────────────────────────────────────────
		{name: "unspecified 0.0.0.0", url: "https://0.0.0.0/repo.git", resolver: publicResolver, wantErr: true},
		{name: "cgnat 100.64.0.1", url: "https://100.64.0.1/repo.git", resolver: publicResolver, wantErr: true},
		{name: "broadcast 255.255.255.255", url: "https://255.255.255.255/repo.git", resolver: publicResolver, wantErr: true},
		{name: "ipv6 ula fc00::1", url: "https://[fc00::1]/repo.git", resolver: publicResolver, wantErr: true},
		{name: "ipv6 ula fd00::1", url: "https://[fd00::1]/repo.git", resolver: publicResolver, wantErr: true},

		// ── hostname resolves to private IP ──────────────────────────────────────
		{name: "hostname resolves to private", url: "https://evil.example.com/repo.git", resolver: privateResolver, wantErr: true},
		{name: "hostname resolves to loopback", url: "https://evil.example.com/repo.git", resolver: loopbackResolver, wantErr: true},

		// ── hostname resolves to public IP ───────────────────────────────────────
		{name: "hostname resolves to public", url: "https://github.com/org/repo.git", resolver: publicResolver},

		// ── hostname unresolvable ─────────────────────────────────────────────────
		{name: "unresolvable hostname", url: "https://nonexistent.invalid/repo.git", resolver: unreachableResolver, wantErr: true},

		// ── docker API (common SSRF target) ──────────────────────────────────────
		{name: "docker api loopback port", url: "http://127.0.0.1:2375/nonexistent.git", resolver: publicResolver, wantErr: true},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			err := validateRepoURLWithResolver(tt.url, tt.resolver)
			if tt.wantErr && err == nil {
				t.Errorf("validateRepoURLWithResolver(%q) = nil, want error", tt.url)
			}
			if !tt.wantErr && err != nil {
				t.Errorf("validateRepoURLWithResolver(%q) = %v, want nil", tt.url, err)
			}
		})
	}
}

// TestValidateRepoURL_ErrorMessages verifies that error strings are informative.
func TestValidateRepoURL_ErrorMessages(t *testing.T) {
	t.Parallel()

	resolver := stubResolver([]string{"140.82.113.4"}, nil)

	cases := []struct {
		url     string
		contain string
	}{
		{"", "empty"},
		{"file:///etc/passwd", "not allowed"},
		{"https://localhost/x", "not allowed"},
		{"https://127.0.0.1/x", "private or reserved"},
		{"https://169.254.169.254/x", "private or reserved"},
	}
	for _, c := range cases {
		c := c
		t.Run(c.url, func(t *testing.T) {
			t.Parallel()
			err := validateRepoURLWithResolver(c.url, resolver)
			if err == nil {
				t.Fatalf("expected error, got nil")
			}
			msg := err.Error()
			if len(msg) < 5 {
				t.Errorf("error message %q is too short to be informative", msg)
			}
			_ = fmt.Sprintf("error for %q: %v", c.url, err) // ensure err is non-nil and printable
		})
	}
}