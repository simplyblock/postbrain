// Command postbrain-cli is the Postbrain CLI for hooks and skill tooling.
package main

import (
	"bufio"
	"bytes"
	"context"
	_ "embed"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/spf13/cobra"

	"github.com/simplyblock/postbrain/internal/closeutil"
	"github.com/simplyblock/postbrain/internal/db"
	"github.com/simplyblock/postbrain/internal/postbraincli"
	"github.com/simplyblock/postbrain/internal/skills"
)

//go:embed assets/codex.md
var embeddedCodexSkillFull string

//go:embed assets/codex-lite.md
var embeddedCodexSkillLight string

//go:embed assets/claude-code.md
var embeddedClaudeSkill string

// Effectively disabled for now since the hooks system is still very minimal
const minimumCodexHooksVersion = "0.114.0"
const latestReleaseAPIURL = "https://api.github.com/repos/simplyblock/postbrain/releases/latest"

var detectCodexVersionFn = detectCodexVersion
var fetchLatestPostbrainVersionFn = fetchLatestPostbrainVersion
var getwdFn = os.Getwd

// hookClient is a minimal HTTP client for the Postbrain REST API.
type hookClient struct {
	baseURL string
	token   string
	http    *http.Client
}

func newHookClient() *hookClient {
	url := resolveURLForRuntime()
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
var buildVersion = "dev"
var buildGitRef = "unknown"
var buildTimestamp = "unknown"

func main() {
	root := newRootCmd()
	if err := root.Execute(); err != nil {
		os.Exit(1)
	}
}

func newRootCmd() *cobra.Command {
	root := &cobra.Command{
		Use:     "postbrain-cli",
		Aliases: []string{"postbrain-hook"},
		Short:   "Postbrain CLI for hooks, skills, and local installer tooling",
	}
	root.PersistentFlags().StringVar(&scopeFlag, "scope", "", "scope (e.g. project:acme/api)")

	root.AddCommand(snapshotCmd(), summarizeSessionCmd(), skillCmd(), installCodexSkillCmd(), installClaudeSkillCmd(), versionCmd(), checkUpdateCmd())
	return root
}

func versionCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "version",
		Short: "Print build version",
		RunE: func(cmd *cobra.Command, args []string) error {
			_, err := fmt.Fprintf(
				cmd.OutOrStdout(),
				"version=%s git=%s built=%s\n",
				buildVersion,
				buildGitRef,
				buildTimestamp,
			)
			return err
		},
	}
}

func checkUpdateCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "check-update",
		Short: "Check whether a newer postbrain-cli release is available",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx := cmd.Context()
			if ctx == nil {
				ctx = context.Background()
			}

			latest, err := fetchLatestPostbrainVersionFn(ctx)
			if err != nil {
				return err
			}
			current := buildVersion
			if strings.TrimSpace(current) == "" || strings.EqualFold(strings.TrimSpace(current), "dev") {
				_, err := fmt.Fprintf(cmd.OutOrStdout(), "current=%s latest=%s (unable to compare dev build)\n", current, latest)
				return err
			}
			cmp, err := compareVersionStrings(latest, current)
			if err != nil {
				return err
			}
			if cmp > 0 {
				_, err := fmt.Fprintf(cmd.OutOrStdout(), "update available: current=%s latest=%s\n", current, latest)
				return err
			}
			if cmp < 0 {
				_, err := fmt.Fprintf(cmd.OutOrStdout(), "current version is ahead of latest release: current=%s latest=%s\n", current, latest)
				return err
			}
			_, err = fmt.Fprintf(cmd.OutOrStdout(), "up to date: current=%s latest=%s\n", current, latest)
			return err
		},
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
	scope := resolveScopeForRuntime()
	if scope == "" {
		slog.Info("snapshot: no scope configured, skipping")
		return nil
	}

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

	normalizedInput := normalizeSnapshotToolInputPaths(hookData.ToolInput)

	// Extract file path from tool input.
	var sourceRef string
	if fp, ok := normalizedInput["file_path"].(string); ok {
		sourceRef = "file:" + fp
	} else if fp, ok := normalizedInput["path"].(string); ok {
		sourceRef = "file:" + fp
	}

	// Build a compact, useful summary for memory content.
	desc := buildSnapshotDescription(hookData.ToolName, normalizedInput, sourceRef)

	// 60s dedup check: query recent memories with same source_ref.
	if sourceRef != "" {
		resp, err := client.get(ctx, fmt.Sprintf("/v1/memories/recall?query=%s&scope=%s&limit=1&min_score=0.99",
			sourceRef, scope))
		if err == nil {
			defer closeutil.Log(resp.Body, "snapshot dedup response body")
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
		"scope":       scope,
	}
	if sourceRef != "" {
		body["source_ref"] = sourceRef
	}

	resp, err := client.post(ctx, "/v1/memories", body)
	if err != nil {
		return fmt.Errorf("snapshot: post memory: %w", err)
	}
	defer closeutil.Log(resp.Body, "snapshot create memory response body")
	slog.Info("snapshot: memory recorded", "tool", hookData.ToolName, "source_ref", sourceRef)
	return nil
}

func buildSnapshotDescription(toolName string, toolInput map[string]any, sourceRef string) string {
	desc := fmt.Sprintf("Tool %s called", toolName)
	if sourceRef != "" {
		desc += " on " + sourceRef
	}

	details := snapshotInputDetails(toolInput)
	if len(details) == 0 {
		return desc
	}
	return fmt.Sprintf("%s (%s)", desc, strings.Join(details, ", "))
}

func snapshotInputDetails(toolInput map[string]any) []string {
	if len(toolInput) == 0 {
		return nil
	}

	details := make([]string, 0, 6)
	added := map[string]struct{}{}
	add := func(key, value string) {
		if strings.TrimSpace(value) == "" {
			return
		}
		if _, ok := added[key]; ok {
			return
		}
		details = append(details, fmt.Sprintf("%s=%s", key, value))
		added[key] = struct{}{}
	}

	if cmd := snapshotStringValue(toolInput["command"]); cmd != "" {
		add("command", snapshotQuote(snapshotTruncateWhitespace(cmd, 120)))
	}
	if cmd := snapshotStringValue(toolInput["cmd"]); cmd != "" {
		add("command", snapshotQuote(snapshotTruncateWhitespace(cmd, 120)))
	}
	if content := snapshotStringValue(toolInput["content"]); content != "" {
		add("content_bytes", strconv.Itoa(len(content)))
	}
	if oldString := snapshotStringValue(toolInput["old_string"]); oldString != "" {
		add("old_string_bytes", strconv.Itoa(len(oldString)))
	}
	if newString := snapshotStringValue(toolInput["new_string"]); newString != "" {
		add("new_string_bytes", strconv.Itoa(len(newString)))
	}

	orderedKeys := []string{
		"file_path", "path", "url", "query", "pattern", "target",
		"line", "lineno", "limit", "id", "name",
	}
	for _, key := range orderedKeys {
		if value, ok := snapshotScalarString(toolInput[key]); ok {
			add(key, snapshotTruncateWhitespace(value, 80))
		}
	}

	if len(details) == 0 {
		add("input_keys", strconv.Itoa(len(toolInput)))
		return details
	}

	// Include a small deterministic tail of scalar keys for extra context.
	var extraKeys []string
	for key := range toolInput {
		if _, ok := added[key]; ok {
			continue
		}
		if _, ok := snapshotScalarString(toolInput[key]); !ok {
			continue
		}
		extraKeys = append(extraKeys, key)
	}
	sort.Strings(extraKeys)
	for _, key := range extraKeys {
		if len(details) >= 6 {
			break
		}
		if value, ok := snapshotScalarString(toolInput[key]); ok {
			add(key, snapshotTruncateWhitespace(value, 80))
		}
	}
	return details
}

func snapshotStringValue(v any) string {
	s, _ := v.(string)
	return s
}

func snapshotScalarString(v any) (string, bool) {
	switch val := v.(type) {
	case string:
		return val, true
	case bool:
		return strconv.FormatBool(val), true
	case float64:
		return strconv.FormatFloat(val, 'f', -1, 64), true
	case int:
		return strconv.Itoa(val), true
	case int64:
		return strconv.FormatInt(val, 10), true
	case json.Number:
		return val.String(), true
	default:
		return "", false
	}
}

func snapshotTruncateWhitespace(s string, max int) string {
	clean := strings.Join(strings.Fields(strings.TrimSpace(s)), " ")
	if max <= 0 || len(clean) <= max {
		return clean
	}
	if max <= 3 {
		return clean[:max]
	}
	return clean[:max-3] + "..."
}

func snapshotQuote(s string) string {
	if s == "" {
		return ""
	}
	return strconv.Quote(s)
}

func normalizeSnapshotToolInputPaths(toolInput map[string]any) map[string]any {
	if len(toolInput) == 0 {
		return toolInput
	}

	out := make(map[string]any, len(toolInput))
	for k, v := range toolInput {
		out[k] = v
	}

	for _, key := range []string{"file_path", "path"} {
		if raw, ok := out[key].(string); ok {
			out[key] = normalizePathToProjectRoot(raw)
		}
	}
	return out
}

func normalizePathToProjectRoot(path string) string {
	path = strings.TrimSpace(path)
	if path == "" {
		return ""
	}

	clean := filepath.Clean(path)
	if !filepath.IsAbs(clean) {
		return filepath.ToSlash(clean)
	}

	cwd, err := getwdFn()
	if err != nil || strings.TrimSpace(cwd) == "" {
		return filepath.ToSlash(clean)
	}

	rel, err := filepath.Rel(cwd, clean)
	if err != nil {
		return filepath.ToSlash(clean)
	}
	if rel == ".." || strings.HasPrefix(rel, ".."+string(os.PathSeparator)) {
		return filepath.ToSlash(clean)
	}
	return filepath.ToSlash(rel)
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
	scope := resolveScopeForRuntime()
	if scope == "" {
		slog.Info("summarize-session: no scope configured, skipping")
		return nil
	}

	// Fetch episodic memories for this session.
	resp, err := client.get(ctx, fmt.Sprintf(
		"/v1/memories/recall?scope=%s&memory_types=episodic&limit=100&min_score=0.0", scope))
	if err != nil {
		return fmt.Errorf("summarize-session: list memories: %w", err)
	}
	defer closeutil.Log(resp.Body, "summarize session list memories response body")

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

	// Call POST /v1/memories/summarize (REST endpoint that proxies consolidation logic).
	summarizeResp, err := client.post(ctx, "/v1/memories/summarize", map[string]any{
		"scope":   scope,
		"topic":   "session summary for " + sessionID,
		"dry_run": false,
	})
	if err != nil {
		return fmt.Errorf("summarize-session: summarize: %w", err)
	}
	defer closeutil.Log(summarizeResp.Body, "summarize session response body")
	slog.Info("summarize-session: session summarized", "scope", scope)
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
	defer closeutil.Log(resp.Body, "skill sync list response body")

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
		skillID, err := parseSkillID(sk.ID)
		if err != nil {
			slog.Warn("skill sync: skipping skill with invalid id", "id", sk.ID, "slug", sk.Slug, "err", err)
			continue
		}
		dbSkill := &db.Skill{
			ID:          skillID,
			Slug:        sk.Slug,
			Name:        sk.Name,
			Description: sk.Description,
			AgentTypes:  sk.AgentTypes,
			Body:        sk.Body,
			Version:     int32(sk.Version),
			Parameters:  sk.Parameters,
		}

		if !skills.IsInstalled(sk.Slug, agentType, workdir) {
			if _, err := skills.Install(dbSkill, agentType, workdir); err != nil {
				slog.Warn("skill sync: install failed", "slug", sk.Slug, "err", err)
				continue
			}
			installed = append(installed, sk.Slug)
		} else if skills.ReadInstalledVersion(sk.Slug, agentType, workdir) != sk.Version {
			if _, err := skills.Install(dbSkill, agentType, workdir); err != nil {
				slog.Warn("skill sync: update failed", "slug", sk.Slug, "err", err)
				continue
			}
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
			defer closeutil.Log(resp.Body, "skill install search response body")

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
				return fmt.Errorf("skill install: decode: %w", err)
			}
			if len(result.Data) == 0 {
				return fmt.Errorf("skill install: slug %q not found in registry", slug)
			}
			sk := result.Data[0]
			skillID, err := parseSkillID(sk.ID)
			if err != nil {
				return fmt.Errorf("skill install: %w", err)
			}
			dbSkill := &db.Skill{
				ID:          skillID,
				Slug:        sk.Slug,
				Name:        sk.Name,
				Description: sk.Description,
				AgentTypes:  sk.AgentTypes,
				Body:        sk.Body,
				Version:     int32(sk.Version),
				Parameters:  sk.Parameters,
			}
			path, err := skills.Install(dbSkill, agentType, workdir)
			if err != nil {
				return fmt.Errorf("skill install: %w", err)
			}
			slog.Info("skill install: installed", "slug", slug, "path", path)
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

// parseSkillID parses a UUID string from the API. An invalid UUID is a data-
// corruption risk: uuid.Parse returns the zero UUID on failure, which would be
// silently written to the database. Return an explicit error instead.
func parseSkillID(raw string) (uuid.UUID, error) {
	id, err := uuid.Parse(raw)
	if err != nil {
		return uuid.UUID{}, fmt.Errorf("skill: invalid id %q: %w", raw, err)
	}
	return id, nil
}

func installCodexSkillCmd() *cobra.Command {
	var targetDir string
	var autoapprove bool
	cmd := &cobra.Command{
		Use:   "install-codex-skill [target_dir]",
		Short: "Install .agents/skills/postbrain/SKILL.md into a target directory",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) > 0 {
				targetDir = args[0]
			}
			if strings.TrimSpace(targetDir) == "" {
				targetDir = "."
			}

			installHooks := runtime.GOOS != "windows"
			enforceCodexVersion := shouldEnforceCodexVersion(runtime.GOOS)
			codexVersion := "not_checked"
			var err error
			if enforceCodexVersion {
				codexVersion, err = detectCodexVersionFn()
				if err != nil {
					return err
				}
				ok, err := codexVersionMeetsMinimum(codexVersion, minimumCodexHooksVersion)
				if err != nil {
					return err
				}
				if !ok {
					return fmt.Errorf("codex version %q is below required minimum %s", codexVersion, minimumCodexHooksVersion)
				}
			}
			if !installHooks {
				slog.Warn("Codex hooks are unavailable on Windows; installing full skill without hooks")
			}
			scope := resolveScopeForInstall(targetDir)
			backendURL, err := resolveURLForInstall(cmd, targetDir)
			if err != nil {
				return err
			}

			installedPath, updatedAgents, err := postbraincli.InstallCodexSkillWithOptions(
				targetDir,
				codexSkillContent(runtime.GOOS),
				backendURL,
				scope,
				postbraincli.CodexSkillInstallOptions{InstallHooks: installHooks},
			)
			if err != nil {
				return err
			}
			updatedConfig := false
			if installHooks {
				updatedConfig, err = postbraincli.EnableCodexHooks(targetDir)
				if err != nil {
					return err
				}
			}
			updatedMCP, err := postbraincli.InstallCodexMCPConfig(targetDir, backendURL)
			if err != nil {
				return err
			}
			updatedPerms := false
			if autoapprove {
				updatedPerms, err = postbraincli.InstallCodexPermissions(targetDir)
				if err != nil {
					return err
				}
			}
			slog.Info("install-codex-skill: installed",
				"path", installedPath,
				"codex_version", codexVersion,
				"backend_url", backendURL,
				"hooks_installed", installHooks,
				"agents_updated", updatedAgents,
				"config_updated", updatedConfig,
				"mcp_updated", updatedMCP,
				"permissions_updated", updatedPerms)
			return nil
		},
	}
	cmd.Flags().StringVar(&targetDir, "target", ".", "target directory")
	cmd.Flags().BoolVar(&autoapprove, "autoapprove", false, "add approval_mode=approve for all Postbrain tools in .codex/config.toml")
	return cmd
}

func codexSkillContent(goos string) string {
	if strings.EqualFold(goos, "windows") {
		return embeddedCodexSkillFull
	}
	return embeddedCodexSkillLight
}

func shouldEnforceCodexVersion(goos string) bool {
	return !strings.EqualFold(goos, "windows")
}

func detectCodexVersion() (string, error) {
	out, err := exec.Command("codex", "--version").CombinedOutput()
	trimmedOut := strings.TrimSpace(string(out))
	if err != nil {
		if trimmedOut != "" {
			return "", fmt.Errorf("run codex --version: %w (output: %q)", err, trimmedOut)
		}
		return "", fmt.Errorf("run codex --version: %w", err)
	}
	return trimmedOut, nil
}

func codexVersionMeetsMinimum(versionOutput, minimum string) (bool, error) {
	cmp, err := compareVersionStrings(versionOutput, minimum)
	if err != nil {
		return false, err
	}
	return cmp >= 0, nil
}

type semver struct {
	major int
	minor int
	patch int
}

func fetchLatestPostbrainVersion(ctx context.Context) (string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, latestReleaseAPIURL, nil)
	if err != nil {
		return "", fmt.Errorf("build latest release request: %w", err)
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("User-Agent", "postbrain-cli")
	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return "", fmt.Errorf("fetch latest release: %w", err)
	}
	defer closeutil.Log(resp.Body, "latest release response body")
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
		return "", fmt.Errorf("fetch latest release: status %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}
	var payload struct {
		TagName string `json:"tag_name"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		return "", fmt.Errorf("decode latest release response: %w", err)
	}
	if strings.TrimSpace(payload.TagName) == "" {
		return "", fmt.Errorf("latest release response missing tag_name")
	}
	return strings.TrimPrefix(strings.TrimSpace(payload.TagName), "v"), nil
}

func compareVersionStrings(a, b string) (int, error) {
	av, err := extractSemver(a)
	if err != nil {
		return 0, err
	}
	bv, err := extractSemver(b)
	if err != nil {
		return 0, err
	}
	return compareSemver(av, bv), nil
}

func compareSemver(a, b semver) int {
	if a.major != b.major {
		if a.major > b.major {
			return 1
		}
		return -1
	}
	if a.minor != b.minor {
		if a.minor > b.minor {
			return 1
		}
		return -1
	}
	if a.patch != b.patch {
		if a.patch > b.patch {
			return 1
		}
		return -1
	}
	return 0
}

func extractSemver(input string) (semver, error) {
	re := regexp.MustCompile(`(\d+)\.(\d+)\.(\d+)`)
	matches := re.FindStringSubmatch(input)
	if len(matches) != 4 {
		return semver{}, fmt.Errorf("could not parse semantic version from %q", input)
	}
	major, err := strconv.Atoi(matches[1])
	if err != nil {
		return semver{}, fmt.Errorf("parse major version: %w", err)
	}
	minor, err := strconv.Atoi(matches[2])
	if err != nil {
		return semver{}, fmt.Errorf("parse minor version: %w", err)
	}
	patch, err := strconv.Atoi(matches[3])
	if err != nil {
		return semver{}, fmt.Errorf("parse patch version: %w", err)
	}
	return semver{major: major, minor: minor, patch: patch}, nil
}

func installClaudeSkillCmd() *cobra.Command {
	var targetDir string
	var autoapprove bool
	cmd := &cobra.Command{
		Use:   "install-claude-skill [target_dir]",
		Short: "Install .claude/skills/postbrain/SKILL.md into a target directory",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) > 0 {
				targetDir = args[0]
			}
			if strings.TrimSpace(targetDir) == "" {
				targetDir = "."
			}
			scope := resolveScopeForInstall(targetDir)
			backendURL, err := resolveURLForInstall(cmd, targetDir)
			if err != nil {
				return err
			}
			installedPath, updatedClaude, err := postbraincli.InstallClaudeSkill(
				targetDir,
				embeddedClaudeSkill,
				backendURL,
				scope,
			)
			if err != nil {
				return err
			}
			updatedSettings, err := postbraincli.InstallClaudeHooks(targetDir, scope)
			if err != nil {
				return err
			}
			updatedMCP, err := postbraincli.InstallClaudeMCPConfig(targetDir, backendURL)
			if err != nil {
				return err
			}
			updatedPerms := false
			if autoapprove {
				updatedPerms, err = postbraincli.InstallClaudePermissions(targetDir)
				if err != nil {
					return err
				}
			}
			slog.Info("install-claude-skill: installed",
				"path", installedPath,
				"backend_url", backendURL,
				"claude_updated", updatedClaude,
				"settings_updated", updatedSettings,
				"mcp_updated", updatedMCP,
				"permissions_updated", updatedPerms)
			return nil
		},
	}
	cmd.Flags().StringVar(&targetDir, "target", ".", "target directory")
	cmd.Flags().BoolVar(&autoapprove, "autoapprove", false, "add mcp__postbrain__* to permissions.allow in .claude/settings.local.json")
	return cmd
}

func resolveScopeForInstall(targetDir string) string {
	if scope := strings.TrimSpace(os.Getenv("POSTBRAIN_SCOPE")); scope != "" {
		return scope
	}
	return postbraincli.ResolveScopeFromBaseFiles(targetDir)
}

func resolveURLForInstall(cmd *cobra.Command, targetDir string) (string, error) {
	if url := strings.TrimSpace(os.Getenv("POSTBRAIN_URL")); url != "" {
		return strings.TrimRight(url, "/"), nil
	}
	if url := strings.TrimSpace(postbraincli.ResolveURLFromBaseFiles(targetDir)); url != "" {
		return strings.TrimRight(url, "/"), nil
	}

	var out io.Writer = os.Stderr
	var in io.Reader = os.Stdin
	if cmd != nil {
		if cmd.ErrOrStderr() != nil {
			out = cmd.ErrOrStderr()
		}
		if cmd.InOrStdin() != nil {
			in = cmd.InOrStdin()
		}
	}

	const defaultURL = "http://localhost:7433"
	_, _ = fmt.Fprintf(out, "Postbrain backend URL [%s]: ", defaultURL)
	reader := bufio.NewReader(in)
	line, err := reader.ReadString('\n')
	if err != nil && err != io.EOF {
		return "", fmt.Errorf("read backend URL: %w", err)
	}
	trimmed := strings.TrimSpace(line)
	if trimmed == "" {
		return defaultURL, nil
	}
	return strings.TrimRight(trimmed, "/"), nil
}

func resolveScopeForRuntime() string {
	if strings.TrimSpace(scopeFlag) != "" {
		return strings.TrimSpace(scopeFlag)
	}
	if scope := strings.TrimSpace(os.Getenv("POSTBRAIN_SCOPE")); scope != "" {
		return scope
	}
	cwd, err := getwdFn()
	if err != nil {
		return ""
	}
	return postbraincli.ResolveScopeFromBaseFiles(cwd)
}

func resolveURLForRuntime() string {
	if url := strings.TrimSpace(os.Getenv("POSTBRAIN_URL")); url != "" {
		return strings.TrimRight(url, "/")
	}
	cwd, err := getwdFn()
	if err == nil {
		if url := strings.TrimSpace(postbraincli.ResolveURLFromBaseFiles(cwd)); url != "" {
			return strings.TrimRight(url, "/")
		}
	}
	return "http://localhost:7433"
}
