package mcp

import (
	"encoding/json"
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

func TestParseSkillMarkdownContent_InvalidInlineAgentTypes_ReturnsError(t *testing.T) {
	t.Parallel()

	content := strings.Join([]string{
		"---",
		"name: Tox Verifier",
		`agent_types: [codex, "claude-code"]`,
		"---",
		"",
		"Body",
	}, "\n")

	_, err := parseSkillMarkdownContent(content)
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !strings.Contains(err.Error(), "invalid agent_types inline list") {
		t.Fatalf("unexpected error: %v", err)
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

func TestDefaultSkillSlug_FromSourceNamePath_SeparatorStable(t *testing.T) {
	t.Parallel()

	gotForward := defaultSkillSlug("", "", "skills/tox-verifier.md")
	if gotForward != "tox-verifier" {
		t.Fatalf("defaultSkillSlug forward = %q, want %q", gotForward, "tox-verifier")
	}

	gotBackward := defaultSkillSlug("", "", `skills\tox-verifier.md`)
	if gotBackward != "tox-verifier" {
		t.Fatalf("defaultSkillSlug backward = %q, want %q", gotBackward, "tox-verifier")
	}
}

func TestParseSkillPublishFiles_Valid(t *testing.T) {
	t.Parallel()

	raw := []any{
		map[string]any{
			"path":       "scripts/run.sh",
			"content":    "#!/bin/sh\necho hi\n",
			"executable": true,
		},
		map[string]any{
			"path":    "references/usage.md",
			"content": "Use it like this.",
		},
	}
	files, err := parseSkillPublishFiles(raw)
	if err != nil {
		t.Fatalf("parseSkillPublishFiles: %v", err)
	}
	if len(files) != 2 {
		t.Fatalf("len(files) = %d, want 2", len(files))
	}
	if !files[0].IsExecutable || files[0].RelativePath != "scripts/run.sh" {
		t.Fatalf("file[0] = %#v", files[0])
	}
	if files[1].IsExecutable || files[1].RelativePath != "references/usage.md" {
		t.Fatalf("file[1] = %#v", files[1])
	}
}

func TestParseSkillPublishFiles_InvalidPath_ReturnsError(t *testing.T) {
	t.Parallel()

	raw := []any{
		map[string]any{
			"path":    "../evil.sh",
			"content": "oops",
		},
	}
	_, err := parseSkillPublishFiles(raw)
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !strings.Contains(err.Error(), "relative_path") && !strings.Contains(err.Error(), "traversal") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestParseSkillPublishFiles_InvalidType_ReturnsError(t *testing.T) {
	t.Parallel()

	_, err := parseSkillPublishFiles([]any{"not-an-object"})
	if err == nil {
		t.Fatal("expected error, got nil")
	}
}

func TestParseSkillPublishFiles_CompatWithJSONDecodeShape(t *testing.T) {
	t.Parallel()

	var decoded struct {
		Files []any `json:"files"`
	}
	if err := json.Unmarshal([]byte(`{
		"files": [
			{"path":"scripts/run.sh","content":"#!/bin/sh","executable":true},
			{"path":"references/usage.md","content":"docs"}
		]
	}`), &decoded); err != nil {
		t.Fatalf("json unmarshal: %v", err)
	}

	files, err := parseSkillPublishFiles(decoded.Files)
	if err != nil {
		t.Fatalf("parseSkillPublishFiles: %v", err)
	}
	if len(files) != 2 {
		t.Fatalf("len(files) = %d, want 2", len(files))
	}
}
