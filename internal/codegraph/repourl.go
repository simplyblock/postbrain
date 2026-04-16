package codegraph

import (
	"errors"
	"fmt"
	"net"
	"net/url"
	"strings"
)

// allowedRepoSchemes lists the URL schemes that are safe for git cloning.
var allowedRepoSchemes = map[string]bool{
	"http":    true,
	"https":   true,
	"git":     true,
	"ssh":     true,
	"git+ssh": true,
}

// blockedCIDRs contains IP ranges that must never be reached via a
// user-supplied repo URL (loopback, link-local, private, reserved).
var blockedCIDRs = func() []*net.IPNet {
	ranges := []string{
		"0.0.0.0/8",          // "this" network
		"10.0.0.0/8",         // RFC-1918 private
		"100.64.0.0/10",      // CGNAT
		"127.0.0.0/8",        // loopback
		"169.254.0.0/16",     // link-local / cloud metadata (AWS IMDSv1, APIPA)
		"172.16.0.0/12",      // RFC-1918 private
		"192.168.0.0/16",     // RFC-1918 private
		"198.18.0.0/15",      // benchmark testing
		"198.51.100.0/24",    // TEST-NET-2
		"203.0.113.0/24",     // TEST-NET-3
		"240.0.0.0/4",        // reserved / future use
		"255.255.255.255/32", // broadcast
		"::1/128",            // IPv6 loopback
		"fc00::/7",           // IPv6 ULA (includes fd00::/8)
		"fe80::/10",          // IPv6 link-local
	}
	nets := make([]*net.IPNet, 0, len(ranges))
	for _, r := range ranges {
		_, n, err := net.ParseCIDR(r)
		if err == nil {
			nets = append(nets, n)
		}
	}
	return nets
}()

// dnsLookup resolves hostnames to IP strings.  It is a package-level variable
// so tests can substitute a stub without real network access.
var dnsLookup = net.LookupHost

// ValidateRepoURL checks that rawURL is safe to use as a git clone destination.
// It rejects dangerous schemes, missing hosts, "localhost", literal private/
// reserved IP addresses, and hostnames that resolve to private addresses.
func ValidateRepoURL(rawURL string) error {
	return validateRepoURLWithResolver(rawURL, dnsLookup)
}

// validateRepoURLWithResolver is the testable core; callers inject the resolver.
func validateRepoURLWithResolver(rawURL string, resolve func(string) ([]string, error)) error {
	if rawURL == "" {
		return errors.New("repo URL must not be empty")
	}

	u, err := url.Parse(rawURL)
	if err != nil {
		return fmt.Errorf("repo URL is not valid: %w", err)
	}

	scheme := strings.ToLower(u.Scheme)
	if !allowedRepoSchemes[scheme] {
		return fmt.Errorf("repo URL scheme %q is not allowed; use https, ssh, or git", u.Scheme)
	}

	host := u.Hostname()
	if host == "" {
		return errors.New("repo URL must include a host")
	}

	// Block "localhost" regardless of what it resolves to on the server.
	if strings.ToLower(host) == "localhost" {
		return fmt.Errorf("repo URL host %q is not allowed", host)
	}

	// If the host is a literal IP address, validate it directly — no DNS needed.
	if ip := net.ParseIP(host); ip != nil {
		return checkIP(ip, host)
	}

	// For hostnames, resolve and check every returned address.  An unresolvable
	// hostname is also rejected: a clone would fail, and we don't permit hosts
	// that cannot be verified.
	addrs, err := resolve(host)
	if err != nil {
		return fmt.Errorf("repo URL host %q cannot be resolved: %w", host, err)
	}
	for _, addr := range addrs {
		ip := net.ParseIP(addr)
		if ip == nil {
			continue
		}
		if err := checkIP(ip, host); err != nil {
			return err
		}
	}
	return nil
}

// checkIP returns an error if ip falls within any blocked CIDR range.
func checkIP(ip net.IP, host string) error {
	for _, cidr := range blockedCIDRs {
		if cidr.Contains(ip) {
			return fmt.Errorf("repo URL host %q resolves to a private or reserved address", host)
		}
	}
	return nil
}
