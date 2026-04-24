package mcp

import (
	"strings"
	"testing"
)

func TestParseSkillMarkdownContent_WithFrontmatter(t *testing.T) {
	t.Parallel()

	content := strings.Join([]string{
		"---",
		"name: Tox Verifier",
		"description: Verify tox output and summarize failures",
		"agent_types: [\"codex\", \"claude-code\"]",
		"---",
		"",
		"Check tox output and report issues.",
	}, "\n")

	draft, err := parseSkillMarkdownContent(content)
	if err != nil {
		t.Fatalf("parseSkillMarkdownContent: %v", err)
	}
	if draft.Name != "Tox Verifier" {
		t.Fatalf("name = %q, want %q", draft.Name, "Tox Verifier")
	}
	if draft.Description != "Verify tox output and summarize failures" {
		t.Fatalf("description = %q, want %q", draft.Description, "Verify tox output and summarize failures")
	}
	if draft.Body != "Check tox output and report issues." {
		t.Fatalf("body = %q, want %q", draft.Body, "Check tox output and report issues.")
	}
	if len(draft.AgentTypes) != 2 || draft.AgentTypes[0] != "codex" || draft.AgentTypes[1] != "claude-code" {
		t.Fatalf("agent_types = %#v, want [codex claude-code]", draft.AgentTypes)
	}
}

func TestParseSkillMarkdownContent_WithYamlAgentTypeList(t *testing.T) {
	t.Parallel()

	content := strings.Join([]string{
		"---",
		"name: Tox Verifier",
		"agent_types:",
		"  - codex",
		"  - claude-code",
		"---",
		"",
		"Body",
	}, "\n")

	draft, err := parseSkillMarkdownContent(content)
	if err != nil {
		t.Fatalf("parseSkillMarkdownContent: %v", err)
	}
	if len(draft.AgentTypes) != 2 || draft.AgentTypes[0] != "codex" || draft.AgentTypes[1] != "claude-code" {
		t.Fatalf("agent_types = %#v, want [codex claude-code]", draft.AgentTypes)
	}
}

func TestParseSkillMarkdownContent_WithoutFrontmatter_UsesWholeBody(t *testing.T) {
	t.Parallel()

	content := "Run tox -e py and summarize failures."
	draft, err := parseSkillMarkdownContent(content)
	if err != nil {
		t.Fatalf("parseSkillMarkdownContent: %v", err)
	}
	if draft.Body != content {
		t.Fatalf("body = %q, want %q", draft.Body, content)
	}
}

func TestDefaultSkillSlug_FromSourceName(t *testing.T) {
	t.Parallel()

	got := defaultSkillSlug("", "", "tox-verifier.md")
	if got != "tox-verifier" {
		t.Fatalf("defaultSkillSlug = %q, want %q", got, "tox-verifier")
	}
}
