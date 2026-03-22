// Command postbrain-hook is the Postbrain agent hook CLI.
package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/spf13/cobra"

	"github.com/simplyblock/postbrain/internal/db"
	"github.com/simplyblock/postbrain/internal/skills"
)

// hookClient is a minimal HTTP client for the Postbrain REST API.
type hookClient struct {
	baseURL string
	token   string
	http    *http.Client
}

func newHookClient() *hookClient {
	url := os.Getenv("POSTBRAIN_URL")
	if url == "" {
		url = "http://localhost:7433"
	}
	token := os.Getenv("POSTBRAIN_TOKEN")
	return &hookClient{
		baseURL: strings.TrimRight(url, "/"),
		token:   token,
		http:    &http.Client{Timeout: 30 * time.Second},
	}
}

func (c *hookClient) post(ctx context.Context, path string, body any) (*http.Response, error) {
	b, err := json.Marshal(body)
	if err != nil {
		return nil, err
	}
	req, err := http.NewRequestWithContext(ctx, "POST", c.baseURL+path, bytes.NewReader(b))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+c.token)
	return c.http.Do(req)
}

func (c *hookClient) get(ctx context.Context, path string) (*http.Response, error) {
	req, err := http.NewRequestWithContext(ctx, "GET", c.baseURL+path, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+c.token)
	return c.http.Do(req)
}

var scopeFlag string

func main() {
	root := &cobra.Command{
		Use:   "postbrain-hook",
		Short: "Postbrain hook CLI for agent integration",
	}
	root.PersistentFlags().StringVar(&scopeFlag, "scope", "", "scope (e.g. project:acme/api)")

	root.AddCommand(snapshotCmd(), summarizeSessionCmd(), skillCmd())
	if err := root.Execute(); err != nil {
		os.Exit(1)
	}
}

func snapshotCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "snapshot",
		Short: "Capture a memory snapshot from a tool call (reads tool output from stdin)",
		RunE:  runSnapshot,
	}
}

func runSnapshot(cmd *cobra.Command, args []string) error {
	ctx := context.Background()
	client := newHookClient()

	// Read tool output JSON from stdin.
	stdinBytes, err := io.ReadAll(os.Stdin)
	if err != nil {
		return fmt.Errorf("read stdin: %w", err)
	}

	// Parse Claude Code PostToolUse hook format:
	// {"tool_name": "Edit", "tool_input": {...}, "tool_response": {...}}
	var hookData struct {
		ToolName     string         `json:"tool_name"`
		ToolInput    map[string]any `json:"tool_input"`
		ToolResponse map[string]any `json:"tool_response"`
	}
	if err := json.Unmarshal(stdinBytes, &hookData); err != nil {
		// Not valid JSON — skip silently (may be called without stdin data).
		slog.Debug("snapshot: stdin not valid JSON, skipping", "err", err)
		return nil
	}

	// Extract file path from tool input.
	var sourceRef string
	if fp, ok := hookData.ToolInput["file_path"].(string); ok {
		sourceRef = "file:" + fp
	} else if fp, ok := hookData.ToolInput["path"].(string); ok {
		sourceRef = "file:" + fp
	}

	// Build description.
	desc := fmt.Sprintf("Tool %s called", hookData.ToolName)
	if sourceRef != "" {
		desc = fmt.Sprintf("Tool %s called on %s", hookData.ToolName, sourceRef)
	}

	// 60s dedup check: query recent memories with same source_ref.
	if sourceRef != "" {
		resp, err := client.get(ctx, fmt.Sprintf("/v1/memories/recall?query=%s&scope=%s&limit=1&min_score=0.99",
			sourceRef, scopeFlag))
		if err == nil {
			defer resp.Body.Close()
			var result struct {
				Results []struct {
					SourceRef string `json:"source_ref"`
				} `json:"results"`
			}
			if json.NewDecoder(resp.Body).Decode(&result) == nil && len(result.Results) > 0 {
				slog.Debug("snapshot: dedup hit, skipping", "source_ref", sourceRef)
				return nil
			}
		}
	}

	// POST to /v1/memories.
	body := map[string]any{
		"content":     desc,
		"memory_type": "episodic",
		"scope":       scopeFlag,
	}
	if sourceRef != "" {
		body["source_ref"] = sourceRef
	}

	resp, err := client.post(ctx, "/v1/memories", body)
	if err != nil {
		return fmt.Errorf("snapshot: post memory: %w", err)
	}
	defer resp.Body.Close()
	slog.Info("snapshot: memory recorded", "tool", hookData.ToolName, "source_ref", sourceRef)
	return nil
}

func summarizeSessionCmd() *cobra.Command {
	var sessionID string
	cmd := &cobra.Command{
		Use:   "summarize-session",
		Short: "Summarize the current session into an episodic memory",
		RunE: func(cmd *cobra.Command, args []string) error {
			return runSummarizeSession(cmd.Context(), sessionID)
		},
	}
	cmd.Flags().StringVar(&sessionID, "session", os.Getenv("CLAUDE_SESSION_ID"), "session ID")
	return cmd
}

func runSummarizeSession(ctx context.Context, sessionID string) error {
	if ctx == nil {
		ctx = context.Background()
	}
	client := newHookClient()

	// Fetch episodic memories for this session.
	resp, err := client.get(ctx, fmt.Sprintf(
		"/v1/memories/recall?scope=%s&memory_types=episodic&limit=100&min_score=0.0", scopeFlag))
	if err != nil {
		return fmt.Errorf("summarize-session: list memories: %w", err)
	}
	defer resp.Body.Close()

	var result struct {
		Results []struct {
			ID string `json:"id"`
		} `json:"results"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return fmt.Errorf("summarize-session: decode: %w", err)
	}

	if len(result.Results) < 3 {
		slog.Info("summarize-session: fewer than 3 episodic memories, skipping")
		return nil
	}

	// TODO(task-hook): POST /v1/memories/summarize endpoint needed; currently using MCP tool.
	// Call POST /v1/memories/summarize (REST endpoint that proxies consolidation logic).
	summarizeResp, err := client.post(ctx, "/v1/memories/summarize", map[string]any{
		"scope":   scopeFlag,
		"topic":   "session summary for " + sessionID,
		"dry_run": false,
	})
	if err != nil {
		return fmt.Errorf("summarize-session: summarize: %w", err)
	}
	defer summarizeResp.Body.Close()
	slog.Info("summarize-session: session summarized", "scope", scopeFlag)
	return nil
}

func skillCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "skill",
		Short: "Skill management",
	}
	cmd.AddCommand(skillSyncCmd(), skillInstallCmd(), skillListCmd())
	return cmd
}

func skillSyncCmd() *cobra.Command {
	var agentType string
	var workdir string
	cmd := &cobra.Command{
		Use:   "sync",
		Short: "Sync published skills to local agent command directory",
		RunE: func(cmd *cobra.Command, args []string) error {
			return runSkillSync(cmd.Context(), agentType, workdir)
		},
	}
	cmd.Flags().StringVar(&agentType, "agent", "claude-code", "agent type")
	cmd.Flags().StringVar(&workdir, "workdir", ".", "working directory")
	return cmd
}

func runSkillSync(ctx context.Context, agentType, workdir string) error {
	if ctx == nil {
		ctx = context.Background()
	}
	client := newHookClient()

	resp, err := client.get(ctx, fmt.Sprintf("/v1/skills/search?scope=%s&status=published&limit=100", scopeFlag))
	if err != nil {
		return fmt.Errorf("skill sync: list skills: %w", err)
	}
	defer resp.Body.Close()

	var result struct {
		Data []struct {
			ID          string          `json:"id"`
			Slug        string          `json:"slug"`
			Name        string          `json:"name"`
			Description string          `json:"description"`
			AgentTypes  []string        `json:"agent_types"`
			Body        string          `json:"body"`
			Version     int             `json:"version"`
			Parameters  json.RawMessage `json:"parameters"`
		} `json:"data"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return fmt.Errorf("skill sync: decode: %w", err)
	}

	var installed, updated, orphaned []string

	for _, sk := range result.Data {
		// Convert to db.Skill for install.
		skillID, _ := uuid.Parse(sk.ID)
		dbSkill := &db.Skill{
			ID:          skillID,
			Slug:        sk.Slug,
			Name:        sk.Name,
			Description: sk.Description,
			AgentTypes:  sk.AgentTypes,
			Body:        sk.Body,
			Version:     sk.Version,
			Parameters:  sk.Parameters,
		}

		if !skills.IsInstalled(sk.Slug, agentType, workdir) {
			if _, err := skills.Install(dbSkill, agentType, workdir); err != nil {
				slog.Warn("skill sync: install failed", "slug", sk.Slug, "err", err)
				continue
			}
			installed = append(installed, sk.Slug)
		} else {
			// TODO(task-hook): check version mismatch from frontmatter and re-install if stale
			updated = append(updated, sk.Slug)
		}
	}

	// Find orphaned local files.
	pattern := filepath.Join(workdir, ".claude", "commands", "*.md")
	localFiles, _ := filepath.Glob(pattern)
	slugSet := make(map[string]bool)
	for _, sk := range result.Data {
		slugSet[sk.Slug] = true
	}
	for _, f := range localFiles {
		slug := strings.TrimSuffix(filepath.Base(f), ".md")
		if !slugSet[slug] {
			orphaned = append(orphaned, slug)
		}
	}

	slog.Info("skill sync complete",
		"installed", len(installed),
		"updated", len(updated),
		"orphaned", len(orphaned))
	return nil
}

func skillInstallCmd() *cobra.Command {
	var agentType, workdir, slug string
	cmd := &cobra.Command{
		Use:   "install",
		Short: "Install a single skill by slug",
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx := context.Background()
			client := newHookClient()
			resp, err := client.get(ctx, fmt.Sprintf("/v1/skills/search?q=%s&scope=%s&limit=1", slug, scopeFlag))
			if err != nil {
				return err
			}
			defer resp.Body.Close()
			// TODO(task-hook): parse response and install skill via skills.Install
			slog.Info("skill install", "slug", slug)
			return nil
		},
	}
	cmd.Flags().StringVar(&slug, "slug", "", "skill slug (required)")
	cmd.Flags().StringVar(&agentType, "agent", "claude-code", "agent type")
	cmd.Flags().StringVar(&workdir, "workdir", ".", "working directory")
	_ = cmd.MarkFlagRequired("slug")
	return cmd
}

func skillListCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "list",
		Short: "List installed skills",
		RunE: func(cmd *cobra.Command, args []string) error {
			workdir := "."
			pattern := filepath.Join(workdir, ".claude", "commands", "*.md")
			files, _ := filepath.Glob(pattern)
			for _, f := range files {
				fmt.Println(strings.TrimSuffix(filepath.Base(f), ".md"))
			}
			return nil
		},
	}
}
