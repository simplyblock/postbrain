package main

import (
	"bytes"
	"strings"
	"testing"
)

func TestRootVersionCommand_PrintsBuildVersion(t *testing.T) {
	t.Parallel()

	old := buildVersion
	oldRef := buildGitRef
	oldTime := buildTimestamp
	buildVersion = "1.2.3-test"
	buildGitRef = "abc1234"
	buildTimestamp = "2026-04-03T14:30:00Z"
	t.Cleanup(func() { buildVersion = old })
	t.Cleanup(func() { buildGitRef = oldRef })
	t.Cleanup(func() { buildTimestamp = oldTime })

	root := newRootCmd()
	root.SetArgs([]string{"version"})

	var out bytes.Buffer
	root.SetOut(&out)
	root.SetErr(&out)

	if err := root.Execute(); err != nil {
		t.Fatalf("execute version command: %v", err)
	}

	got := strings.TrimSpace(out.String())
	want := "version=1.2.3-test git=abc1234 built=2026-04-03T14:30:00Z"
	if got != want {
		t.Fatalf("version output = %q, want %q", got, want)
	}
}
