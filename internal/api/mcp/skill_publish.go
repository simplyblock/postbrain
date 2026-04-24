package mcp

import (
	"context"
	"encoding/json"
	"fmt"
	"path"
	"strings"
	"time"

	"github.com/google/uuid"
	mcpgo "github.com/mark3labs/mcp-go/mcp"

	"github.com/simplyblock/postbrain/internal/auth"
	"github.com/simplyblock/postbrain/internal/db"
	"github.com/simplyblock/postbrain/internal/skills"
)

func (s *Server) registerSkillPublish() {
	s.mcpServer.AddTool(mcpgo.NewTool("skill_publish",
		mcpgo.WithReadOnlyHintAnnotation(false),
		mcpgo.WithDestructiveHintAnnotation(false),
		mcpgo.WithOpenWorldHintAnnotation(false),
		mcpgo.WithDescription("Publish a skill to the registry from SKILL.md-style markdown or explicit fields"),
		mcpgo.WithString("scope", mcpgo.Required(), mcpgo.Description("Scope as kind:external_id")),
		mcpgo.WithString("content", mcpgo.Description("Raw skill markdown; supports optional YAML frontmatter")),
		mcpgo.WithString("body", mcpgo.Description("Skill body (used when content is omitted)")),
		mcpgo.WithString("slug", mcpgo.Description("Skill slug; if omitted derived from source_name or name")),
		mcpgo.WithString("name", mcpgo.Description("Skill name; if omitted derived from frontmatter or slug")),
		mcpgo.WithString("description", mcpgo.Description("Short description; if omitted derived from frontmatter or name")),
		mcpgo.WithString("source_name", mcpgo.Description("Optional source filename, e.g. tox-verifier.md")),
		mcpgo.WithString("visibility", mcpgo.Description("private|project|team|department|company (default: team)")),
		mcpgo.WithArray("agent_types", mcpgo.Description("Compatible agent types (default: [\"any\"])"),
			mcpgo.Items(map[string]any{"type": "string"}),
		),
		mcpgo.WithArray("files", mcpgo.Description("Optional supplementary files, e.g. scripts/* and references/*.md"),
			mcpgo.Items(map[string]any{
				"type": "object",
				"properties": map[string]any{
					"path":       map[string]any{"type": "string"},
					"content":    map[string]any{"type": "string"},
					"executable": map[string]any{"type": "boolean"},
				},
				"required": []string{"path", "content"},
			}),
		),
	), withToolMetrics("skill_publish", withToolPermission("skills:write", s.handleSkillPublish)))
}

type skillDraft struct {
	Slug        string
	Name        string
	Description string
	Body        string
	AgentTypes  []string
}

func (s *Server) handleSkillPublish(ctx context.Context, req mcpgo.CallToolRequest) (*mcpgo.CallToolResult, error) {
	args := req.GetArguments()

	scopeStr := argString(args, "scope")
	if scopeStr == "" {
		return mcpgo.NewToolResultError("skill_publish: 'scope' is required"), nil
	}
	content := argString(args, "content")
	body := argString(args, "body")
	if content == "" && body == "" {
		return mcpgo.NewToolResultError("skill_publish: 'content' or 'body' is required"), nil
	}

	if s.pool == nil || s.sklStore == nil {
		return mcpgo.NewToolResultError("skill_publish: server not configured"), nil
	}

	scopeID, errResult := s.resolveScope(ctx, "skill_publish", scopeStr)
	if errResult != nil {
		return errResult, nil
	}

	draft := skillDraft{
		Slug:        strings.TrimSpace(argString(args, "slug")),
		Name:        strings.TrimSpace(argString(args, "name")),
		Description: strings.TrimSpace(argString(args, "description")),
		Body:        strings.TrimSpace(body),
		AgentTypes:  argStringSlice(args, "agent_types"),
	}
	if content != "" {
		parsed, err := parseSkillMarkdownContent(content)
		if err != nil {
			return mcpgo.NewToolResultError(fmt.Sprintf("skill_publish: parse content: %v", err)), nil
		}
		if draft.Name == "" {
			draft.Name = parsed.Name
		}
		if draft.Description == "" {
			draft.Description = parsed.Description
		}
		if draft.Body == "" {
			draft.Body = strings.TrimSpace(parsed.Body)
		}
		if len(draft.AgentTypes) == 0 {
			draft.AgentTypes = parsed.AgentTypes
		}
	}

	draft.Slug = defaultSkillSlug(draft.Slug, draft.Name, argString(args, "source_name"))
	if draft.Slug == "" {
		return mcpgo.NewToolResultError("skill_publish: could not derive slug; provide 'slug', 'name', or 'source_name'"), nil
	}
	if err := skills.ValidateSlug(draft.Slug); err != nil {
		return mcpgo.NewToolResultError(fmt.Sprintf("skill_publish: invalid slug: %v", err)), nil
	}

	if draft.Name == "" {
		draft.Name = titleFromSlug(draft.Slug)
	}
	if draft.Description == "" {
		draft.Description = draft.Name
	}
	if draft.Body == "" {
		return mcpgo.NewToolResultError("skill_publish: body must not be empty"), nil
	}

	visibility := strings.TrimSpace(argString(args, "visibility"))
	if visibility == "" {
		visibility = "team"
	}

	authorID, _ := ctx.Value(auth.ContextKeyPrincipalID).(uuid.UUID)
	if authorID == uuid.Nil {
		return mcpgo.NewToolResultError("skill_publish: missing caller principal"), nil
	}

	var files []db.SkillFileInput
	if rawFiles, ok := args["files"]; ok {
		items, ok := rawFiles.([]any)
		if !ok {
			return mcpgo.NewToolResultError("skill_publish: 'files' must be an array"), nil
		}
		parsedFiles, err := parseSkillPublishFiles(items)
		if err != nil {
			return mcpgo.NewToolResultError(fmt.Sprintf("skill_publish: files: %v", err)), nil
		}
		files = parsedFiles
	}

	created, err := s.sklStore.Create(ctx, skills.CreateInput{
		ScopeID:        scopeID,
		AuthorID:       authorID,
		Slug:           draft.Slug,
		Name:           draft.Name,
		Description:    draft.Description,
		AgentTypes:     draft.AgentTypes,
		Body:           draft.Body,
		Visibility:     visibility,
		Parameters:     nil,
		Files:          files,
		Status:         "published",
		PublishedAt:    ptrTime(time.Now().UTC()),
		ReviewRequired: 1,
	})
	if err != nil {
		return mcpgo.NewToolResultError(fmt.Sprintf("skill_publish: create: %v", err)), nil
	}

	out, _ := json.Marshal(map[string]any{
		"skill_id": created.ID.String(),
		"slug":     created.Slug,
		"status":   created.Status,
		"version":  created.Version,
	})
	return mcpgo.NewToolResultText(string(out)), nil
}

func parseSkillMarkdownContent(raw string) (skillDraft, error) {
	raw = strings.ReplaceAll(raw, "\r\n", "\n")
	if !strings.HasPrefix(raw, "---\n") {
		return skillDraft{Body: raw}, nil
	}

	rest := strings.TrimPrefix(raw, "---\n")
	idx := strings.Index(rest, "\n---\n")
	delimiterLen := len("\n---\n")
	if idx < 0 && strings.HasSuffix(rest, "\n---") {
		idx = len(rest) - len("\n---")
		delimiterLen = len("\n---")
	}
	if idx < 0 {
		return skillDraft{}, fmt.Errorf("frontmatter opening found but closing '---' is missing")
	}

	fm := rest[:idx]
	body := rest[idx+delimiterLen:]
	body = strings.TrimPrefix(body, "\n")
	draft := skillDraft{Body: body}

	lines := strings.Split(fm, "\n")
	inAgentTypes := false
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" {
			continue
		}
		if inAgentTypes {
			if strings.HasPrefix(trimmed, "- ") {
				v := strings.TrimSpace(strings.TrimPrefix(trimmed, "- "))
				if v != "" {
					draft.AgentTypes = append(draft.AgentTypes, v)
				}
				continue
			}
			inAgentTypes = false
		}

		switch {
		case strings.HasPrefix(trimmed, "name:"):
			draft.Name = strings.TrimSpace(strings.TrimPrefix(trimmed, "name:"))
		case strings.HasPrefix(trimmed, "description:"):
			draft.Description = strings.TrimSpace(strings.TrimPrefix(trimmed, "description:"))
		case strings.HasPrefix(trimmed, "agent_types:"):
			rawValue := strings.TrimSpace(strings.TrimPrefix(trimmed, "agent_types:"))
			if rawValue == "" {
				inAgentTypes = true
				continue
			}
			if strings.HasPrefix(rawValue, "[") {
				var parsed []string
				if err := json.Unmarshal([]byte(rawValue), &parsed); err != nil {
					return skillDraft{}, fmt.Errorf("invalid agent_types inline list: %w", err)
				}
				draft.AgentTypes = append(draft.AgentTypes, parsed...)
			}
		}
	}

	return draft, nil
}

func defaultSkillSlug(slug, name, sourceName string) string {
	if normalized := normalizeSlugCandidate(slug); normalized != "" {
		return normalized
	}
	sourceName = strings.ReplaceAll(sourceName, "\\", "/")
	if normalized := normalizeSlugCandidate(strings.TrimSuffix(path.Base(sourceName), path.Ext(sourceName))); normalized != "" {
		return normalized
	}
	return normalizeSlugCandidate(name)
}

func normalizeSlugCandidate(raw string) string {
	raw = strings.ToLower(strings.TrimSpace(raw))
	if raw == "" {
		return ""
	}
	var b strings.Builder
	lastDash := false
	for _, r := range raw {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') {
			b.WriteRune(r)
			lastDash = false
			continue
		}
		if r == '-' || r == '_' {
			b.WriteRune(r)
			lastDash = false
			continue
		}
		if !lastDash {
			b.WriteRune('-')
			lastDash = true
		}
	}
	out := strings.Trim(b.String(), "-_")
	return out
}

func titleFromSlug(slug string) string {
	parts := strings.FieldsFunc(slug, func(r rune) bool { return r == '-' || r == '_' })
	for i, p := range parts {
		if p == "" {
			continue
		}
		parts[i] = strings.ToUpper(p[:1]) + p[1:]
	}
	if len(parts) == 0 {
		return slug
	}
	return strings.Join(parts, " ")
}

func parseSkillPublishFiles(raw []any) ([]db.SkillFileInput, error) {
	files := make([]db.SkillFileInput, 0, len(raw))
	for i, item := range raw {
		obj, ok := item.(map[string]any)
		if !ok {
			return nil, fmt.Errorf("item %d must be an object", i)
		}
		path, ok := obj["path"].(string)
		if !ok || strings.TrimSpace(path) == "" {
			return nil, fmt.Errorf("item %d: path is required", i)
		}
		content, ok := obj["content"].(string)
		if !ok {
			return nil, fmt.Errorf("item %d: content must be a string", i)
		}
		executable, _ := obj["executable"].(bool)
		f := db.SkillFileInput{
			RelativePath: strings.TrimSpace(path),
			Content:      content,
			IsExecutable: executable,
		}
		if err := skills.ValidateSkillFile(f); err != nil {
			return nil, err
		}
		files = append(files, f)
	}
	return files, nil
}

func ptrTime(t time.Time) *time.Time {
	return &t
}
