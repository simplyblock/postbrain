// Command postbrain-cli is the Postbrain CLI for hooks and skill tooling.
package main

import (
	"bytes"
	"context"
	_ "embed"
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
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/spf13/cobra"

	"github.com/simplyblock/postbrain/internal/closeutil"
	"github.com/simplyblock/postbrain/internal/config"
	"github.com/simplyblock/postbrain/internal/db"
	"github.com/simplyblock/postbrain/internal/postbraincli"
	"github.com/simplyblock/postbrain/internal/skills"
)

//go:embed assets/codex.md
var embeddedCodexSkill string

//go:embed assets/claude-code.md
var embeddedClaudeSkill string

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
var buildVersion = "dev"
var buildGitRef = "unknown"
var buildTimestamp = "unknown"

type embeddingModelRegisterOptions struct {
	DatabaseURL   string
	ConfigPath    string
	Slug          string
	Provider      string
	ServiceURL    string
	ProviderModel string
	Dimensions    int
	ContentType   string
	Activate      bool
}

type embeddingModelActivateOptions struct {
	DatabaseURL string
	ConfigPath  string
	Slug        string
	ContentType string
}

type embeddingModelListOptions struct {
	DatabaseURL string
	ConfigPath  string
}

var registerEmbeddingModelCmdFn = runRegisterEmbeddingModelCommand
var activateEmbeddingModelCmdFn = runActivateEmbeddingModelCommand
var listEmbeddingModelCmdFn = runListEmbeddingModelsCommand

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

	root.AddCommand(snapshotCmd(), summarizeSessionCmd(), skillCmd(), embeddingModelCmd(), installCodexSkillCmd(), installClaudeSkillCmd(), versionCmd())
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
		"scope":       scopeFlag,
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
		"scope":   scopeFlag,
		"topic":   "session summary for " + sessionID,
		"dry_run": false,
	})
	if err != nil {
		return fmt.Errorf("summarize-session: summarize: %w", err)
	}
	defer closeutil.Log(summarizeResp.Body, "summarize session response body")
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
	cmd := &cobra.Command{
		Use:   "install-codex-skill [target_dir]",
		Short: "Install .codex/skills/postbrain.md into a target directory",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) > 0 {
				targetDir = args[0]
			}
			if strings.TrimSpace(targetDir) == "" {
				targetDir = "."
			}
			installedPath, updatedAgents, err := postbraincli.InstallCodexSkill(
				targetDir,
				embeddedCodexSkill,
				os.Getenv("POSTBRAIN_URL"),
				os.Getenv("POSTBRAIN_SCOPE"),
			)
			if err != nil {
				return err
			}
			slog.Info("install-codex-skill: installed", "path", installedPath, "agents_updated", updatedAgents)
			return nil
		},
	}
	cmd.Flags().StringVar(&targetDir, "target", ".", "target directory")
	return cmd
}

func installClaudeSkillCmd() *cobra.Command {
	var targetDir string
	cmd := &cobra.Command{
		Use:   "install-claude-skill [target_dir]",
		Short: "Install .claude/postbrain.md into a target directory",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) > 0 {
				targetDir = args[0]
			}
			if strings.TrimSpace(targetDir) == "" {
				targetDir = "."
			}
			installedPath, updatedClaude, err := postbraincli.InstallClaudeSkill(
				targetDir,
				embeddedClaudeSkill,
				os.Getenv("POSTBRAIN_URL"),
				os.Getenv("POSTBRAIN_SCOPE"),
			)
			if err != nil {
				return err
			}
			slog.Info("install-claude-skill: installed", "path", installedPath, "claude_updated", updatedClaude)
			return nil
		},
	}
	cmd.Flags().StringVar(&targetDir, "target", ".", "target directory")
	return cmd
}

func embeddingModelCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "embedding-model",
		Short: "Embedding model management",
	}
	cmd.AddCommand(embeddingModelRegisterCmd(), embeddingModelActivateCmd(), embeddingModelListCmd())
	return cmd
}

func embeddingModelRegisterCmd() *cobra.Command {
	opts := embeddingModelRegisterOptions{}
	cmd := &cobra.Command{
		Use:   "register",
		Short: "Register an embedding model and provision its storage table",
		RunE: func(cmd *cobra.Command, args []string) error {
			msg, err := registerEmbeddingModelCmdFn(cmd.Context(), opts)
			if err != nil {
				return err
			}
			_, err = fmt.Fprintln(cmd.OutOrStdout(), msg)
			return err
		},
	}
	cmd.Flags().StringVar(&opts.DatabaseURL, "database-url", "", "PostgreSQL URL (overrides config file and POSTBRAIN_DATABASE_URL)")
	cmd.Flags().StringVar(&opts.ConfigPath, "config", "config.yaml", "path to config file")
	cmd.Flags().StringVar(&opts.Slug, "slug", "", "model slug (required)")
	cmd.Flags().StringVar(&opts.Provider, "provider", "", "embedding provider, e.g. openai or ollama (required)")
	cmd.Flags().StringVar(&opts.ServiceURL, "service-url", "", "embedding service URL (required)")
	cmd.Flags().StringVar(&opts.ProviderModel, "provider-model", "", "provider-side model name (required)")
	cmd.Flags().IntVar(&opts.Dimensions, "dimensions", 0, "embedding vector dimensions (required)")
	cmd.Flags().StringVar(&opts.ContentType, "content-type", "", "content type: text or code (required)")
	cmd.Flags().BoolVar(&opts.Activate, "activate", false, "set as active model for this content type")
	_ = cmd.MarkFlagRequired("slug")
	_ = cmd.MarkFlagRequired("provider")
	_ = cmd.MarkFlagRequired("service-url")
	_ = cmd.MarkFlagRequired("provider-model")
	_ = cmd.MarkFlagRequired("dimensions")
	_ = cmd.MarkFlagRequired("content-type")
	return cmd
}

func embeddingModelActivateCmd() *cobra.Command {
	opts := embeddingModelActivateOptions{}
	cmd := &cobra.Command{
		Use:   "activate",
		Short: "Activate a registered embedding model for one content type",
		RunE: func(cmd *cobra.Command, args []string) error {
			msg, err := activateEmbeddingModelCmdFn(cmd.Context(), opts)
			if err != nil {
				return err
			}
			_, err = fmt.Fprintln(cmd.OutOrStdout(), msg)
			return err
		},
	}
	cmd.Flags().StringVar(&opts.DatabaseURL, "database-url", "", "PostgreSQL URL (overrides config file and POSTBRAIN_DATABASE_URL)")
	cmd.Flags().StringVar(&opts.ConfigPath, "config", "config.yaml", "path to config file")
	cmd.Flags().StringVar(&opts.Slug, "slug", "", "model slug (required)")
	cmd.Flags().StringVar(&opts.ContentType, "content-type", "", "content type: text or code (required)")
	_ = cmd.MarkFlagRequired("slug")
	_ = cmd.MarkFlagRequired("content-type")
	return cmd
}

func embeddingModelListCmd() *cobra.Command {
	opts := embeddingModelListOptions{}
	cmd := &cobra.Command{
		Use:   "list",
		Short: "List registered embedding models",
		RunE: func(cmd *cobra.Command, args []string) error {
			msg, err := listEmbeddingModelCmdFn(cmd.Context(), opts)
			if err != nil {
				return err
			}
			_, err = fmt.Fprintln(cmd.OutOrStdout(), msg)
			return err
		},
	}
	cmd.Flags().StringVar(&opts.DatabaseURL, "database-url", "", "PostgreSQL URL (overrides config file and POSTBRAIN_DATABASE_URL)")
	cmd.Flags().StringVar(&opts.ConfigPath, "config", "config.yaml", "path to config file")
	return cmd
}

func runRegisterEmbeddingModelCommand(ctx context.Context, opts embeddingModelRegisterOptions) (string, error) {
	pool, err := openCLIPool(ctx, opts.DatabaseURL, opts.ConfigPath)
	if err != nil {
		return "", err
	}
	defer pool.Close()

	model, err := db.RegisterEmbeddingModel(ctx, pool, db.RegisterEmbeddingModelParams{
		Slug:          opts.Slug,
		Provider:      opts.Provider,
		ServiceURL:    opts.ServiceURL,
		ProviderModel: opts.ProviderModel,
		Dimensions:    opts.Dimensions,
		ContentType:   opts.ContentType,
		Activate:      opts.Activate,
	})
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("registered model %s", model.Slug), nil
}

func runActivateEmbeddingModelCommand(ctx context.Context, opts embeddingModelActivateOptions) (string, error) {
	if opts.ContentType != "text" && opts.ContentType != "code" {
		return "", fmt.Errorf("invalid content type %q", opts.ContentType)
	}

	pool, err := openCLIPool(ctx, opts.DatabaseURL, opts.ConfigPath)
	if err != nil {
		return "", err
	}
	defer pool.Close()

	tx, err := pool.Begin(ctx)
	if err != nil {
		return "", fmt.Errorf("begin tx: %w", err)
	}
	defer tx.Rollback(ctx) //nolint:errcheck

	if _, err := tx.Exec(ctx, `
		UPDATE embedding_models
		SET is_active = false
		WHERE content_type = $1
	`, opts.ContentType); err != nil {
		return "", fmt.Errorf("deactivate models: %w", err)
	}

	tag, err := tx.Exec(ctx, `
		UPDATE embedding_models
		SET is_active = true
		WHERE slug = $1 AND content_type = $2
	`, opts.Slug, opts.ContentType)
	if err != nil {
		return "", fmt.Errorf("activate model: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return "", fmt.Errorf("model %q with content type %q not found", opts.Slug, opts.ContentType)
	}

	if err := tx.Commit(ctx); err != nil {
		return "", fmt.Errorf("commit activation: %w", err)
	}
	return fmt.Sprintf("activated model %s for %s", opts.Slug, opts.ContentType), nil
}

func runListEmbeddingModelsCommand(ctx context.Context, opts embeddingModelListOptions) (string, error) {
	pool, err := openCLIPool(ctx, opts.DatabaseURL, opts.ConfigPath)
	if err != nil {
		return "", err
	}
	defer pool.Close()

	rows, err := pool.Query(ctx, `
		SELECT slug, provider, provider_model, content_type, dimensions, is_active, is_ready, COALESCE(table_name, '')
		FROM embedding_models
		ORDER BY content_type, slug
	`)
	if err != nil {
		return "", fmt.Errorf("list embedding models: %w", err)
	}
	defer rows.Close()

	var b strings.Builder
	b.WriteString("slug\tprovider\tprovider_model\tcontent_type\tdimensions\tactive\tready\ttable_name\n")
	for rows.Next() {
		var (
			slug          string
			provider      *string
			providerModel *string
			contentType   string
			dimensions    int
			isActive      bool
			isReady       bool
			tableName     string
		)
		if err := rows.Scan(&slug, &provider, &providerModel, &contentType, &dimensions, &isActive, &isReady, &tableName); err != nil {
			return "", fmt.Errorf("scan embedding model: %w", err)
		}
		b.WriteString(fmt.Sprintf("%s\t%s\t%s\t%s\t%d\t%t\t%t\t%s\n",
			slug, strOrEmpty(provider), strOrEmpty(providerModel), contentType, dimensions, isActive, isReady, tableName))
	}
	if err := rows.Err(); err != nil {
		return "", fmt.Errorf("iterate embedding models: %w", err)
	}
	return strings.TrimSpace(b.String()), nil
}

func openCLIPool(ctx context.Context, databaseURL, configPath string) (*pgxpool.Pool, error) {
	url := strings.TrimSpace(databaseURL)
	if url == "" {
		cfgPath := strings.TrimSpace(configPath)
		if cfgPath == "" {
			cfgPath = "config.yaml"
		}
		var err error
		url, err = config.LoadDatabaseURL(cfgPath)
		if err != nil {
			return nil, fmt.Errorf("load database url: %w", err)
		}
	}

	return db.NewPool(ctx, &config.DatabaseConfig{
		URL:            url,
		MaxOpen:        5,
		MaxIdle:        2,
		ConnectTimeout: 15 * time.Second,
	})
}

func strOrEmpty(v *string) string {
	if v == nil {
		return ""
	}
	return *v
}
