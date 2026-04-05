package main

import (
	"bytes"
	"strings"
	"testing"

	"github.com/google/uuid"
)

func TestParseSkillID_ValidUUID_ReturnsID(t *testing.T) {
	t.Parallel()
	want := uuid.New()
	got, err := parseSkillID(want.String())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != want {
		t.Errorf("got %v, want %v", got, want)
	}
}

func TestParseSkillID_InvalidUUID_ReturnsError(t *testing.T) {
	t.Parallel()
	_, err := parseSkillID("not-a-uuid")
	if err == nil {
		t.Fatal("expected error for invalid UUID, got nil")
	}
}

func TestParseSkillID_EmptyString_ReturnsError(t *testing.T) {
	t.Parallel()
	_, err := parseSkillID("")
	if err == nil {
		t.Fatal("expected error for empty string, got nil")
	}
}

func TestRootVersionCommand_PrintsBuildVersion(t *testing.T) {
	t.Parallel()

	old := buildVersion
	oldRef := buildGitRef
	oldTime := buildTimestamp
	buildVersion = "9.8.7-test"
	buildGitRef = "def5678"
	buildTimestamp = "2026-04-03T14:31:00Z"
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
	want := "version=9.8.7-test git=def5678 built=2026-04-03T14:31:00Z"
	if got != want {
		t.Fatalf("version output = %q, want %q", got, want)
	}
}
