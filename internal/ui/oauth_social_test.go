package ui

import "testing"

func TestIsAllowedEmailDomain(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		email    string
		hd       string
		allowed  []string
		expected bool
	}{
		{name: "allow all when empty", email: "a@b.com", allowed: nil, expected: true},
		{name: "email domain match", email: "dev@example.com", allowed: []string{"example.com"}, expected: true},
		{name: "hosted domain match", email: "dev@other.com", hd: "example.com", allowed: []string{"example.com"}, expected: true},
		{name: "case insensitive", email: "dev@Example.com", allowed: []string{"example.com"}, expected: true},
		{name: "denied mismatch", email: "dev@other.com", allowed: []string{"example.com"}, expected: false},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got := isAllowedEmailDomain(tc.email, tc.hd, tc.allowed)
			if got != tc.expected {
				t.Fatalf("isAllowedEmailDomain(%q,%q,%v)=%v, want %v", tc.email, tc.hd, tc.allowed, got, tc.expected)
			}
		})
	}
}
