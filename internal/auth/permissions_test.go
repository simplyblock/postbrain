package auth

import "testing"

func TestHasReadPermission(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name        string
		permissions []string
		want        bool
	}{
		{name: "no permissions", permissions: nil, want: false},
		{name: "read", permissions: []string{"read"}, want: true},
		{name: "write implies read", permissions: []string{"write"}, want: true},
		{name: "admin implies read", permissions: []string{"admin"}, want: true},
		{name: "domain read", permissions: []string{"memories:read"}, want: true},
		{name: "domain write implies read", permissions: []string{"knowledge:write"}, want: true},
	}
	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got := HasReadPermission(tc.permissions)
			if got != tc.want {
				t.Fatalf("HasReadPermission(%v) = %v, want %v", tc.permissions, got, tc.want)
			}
		})
	}
}

func TestHasWritePermission(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name        string
		permissions []string
		want        bool
	}{
		{name: "no permissions", permissions: nil, want: false},
		{name: "read only", permissions: []string{"read"}, want: false},
		{name: "write", permissions: []string{"write"}, want: true},
		{name: "admin", permissions: []string{"admin"}, want: true},
		{name: "domain read only", permissions: []string{"skills:read"}, want: false},
		{name: "domain write", permissions: []string{"skills:write"}, want: true},
	}
	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got := HasWritePermission(tc.permissions)
			if got != tc.want {
				t.Fatalf("HasWritePermission(%v) = %v, want %v", tc.permissions, got, tc.want)
			}
		})
	}
}
