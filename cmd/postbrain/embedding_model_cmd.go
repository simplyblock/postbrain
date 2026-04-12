package main

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"

	"github.com/simplyblock/postbrain/internal/config"
	"github.com/simplyblock/postbrain/internal/db"
)

type embeddingModelRegisterOptions struct {
	DatabaseURL    string
	Slug           string
	ProviderConfig string
	Dimensions     int
	ContentType    string
	Activate       bool
}

type resolvedEmbeddingRegistration struct {
	Provider       string
	ServiceURL     string
	ProviderModel  string
	ProviderConfig string
}

type embeddingModelActivateOptions struct {
	DatabaseURL string
	Slug        string
	ContentType string
}

type embeddingModelListOptions struct {
	DatabaseURL string
}

var registerEmbeddingModelCmdFn = runRegisterEmbeddingModelCommand
var activateEmbeddingModelCmdFn = runActivateEmbeddingModelCommand
var listEmbeddingModelCmdFn = runListEmbeddingModelsCommand
var registerSummaryModelCmdFn = runRegisterSummaryModelCommand
var activateSummaryModelCmdFn = runActivateSummaryModelCommand
var listSummaryModelCmdFn = runListSummaryModelsCommand

func embeddingModelCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "embedding-model",
		Short: "Embedding model management",
	}
	cmd.AddCommand(embeddingModelRegisterCmd(), embeddingModelActivateCmd(), embeddingModelListCmd())
	return cmd
}

func summaryModelCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "summary-model",
		Short: "Summary model management",
	}
	cmd.AddCommand(summaryModelRegisterCmd(), summaryModelActivateCmd(), summaryModelListCmd())
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
	cmd.Flags().StringVar(&opts.Slug, "slug", "", "model slug (required)")
	cmd.Flags().StringVar(&opts.ProviderConfig, "provider-config", "default", "named embedding provider profile to use")
	cmd.Flags().IntVar(&opts.Dimensions, "dimensions", 0, "embedding vector dimensions (required)")
	cmd.Flags().StringVar(&opts.ContentType, "content-type", "", "content type: text or code (required)")
	cmd.Flags().BoolVar(&opts.Activate, "activate", false, "set as active model for this content type")
	_ = cmd.MarkFlagRequired("slug")
	_ = cmd.MarkFlagRequired("dimensions")
	_ = cmd.MarkFlagRequired("content-type")
	return cmd
}

func summaryModelRegisterCmd() *cobra.Command {
	opts := embeddingModelRegisterOptions{ContentType: "text"}
	cmd := &cobra.Command{
		Use:   "register",
		Short: "Register a summary generation model",
		RunE: func(cmd *cobra.Command, args []string) error {
			opts.ContentType = "text"
			if opts.Dimensions <= 0 {
				opts.Dimensions = 1
			}
			msg, err := registerSummaryModelCmdFn(cmd.Context(), opts)
			if err != nil {
				return err
			}
			_, err = fmt.Fprintln(cmd.OutOrStdout(), msg)
			return err
		},
	}
	cmd.Flags().StringVar(&opts.DatabaseURL, "database-url", "", "PostgreSQL URL (overrides config file and POSTBRAIN_DATABASE_URL)")
	cmd.Flags().StringVar(&opts.Slug, "slug", "", "model slug (required)")
	cmd.Flags().StringVar(&opts.ProviderConfig, "provider-config", "default", "named embedding provider profile to use")
	cmd.Flags().BoolVar(&opts.Activate, "activate", false, "set as active summary model")
	_ = cmd.MarkFlagRequired("slug")
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
	cmd.Flags().StringVar(&opts.Slug, "slug", "", "model slug (required)")
	cmd.Flags().StringVar(&opts.ContentType, "content-type", "", "content type: text or code (required)")
	_ = cmd.MarkFlagRequired("slug")
	_ = cmd.MarkFlagRequired("content-type")
	return cmd
}

func summaryModelActivateCmd() *cobra.Command {
	opts := embeddingModelActivateOptions{ContentType: "text"}
	cmd := &cobra.Command{
		Use:   "activate",
		Short: "Activate a registered summary model",
		RunE: func(cmd *cobra.Command, args []string) error {
			opts.ContentType = "text"
			msg, err := activateSummaryModelCmdFn(cmd.Context(), opts)
			if err != nil {
				return err
			}
			_, err = fmt.Fprintln(cmd.OutOrStdout(), msg)
			return err
		},
	}
	cmd.Flags().StringVar(&opts.DatabaseURL, "database-url", "", "PostgreSQL URL (overrides config file and POSTBRAIN_DATABASE_URL)")
	cmd.Flags().StringVar(&opts.Slug, "slug", "", "model slug (required)")
	_ = cmd.MarkFlagRequired("slug")
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
	return cmd
}

func summaryModelListCmd() *cobra.Command {
	opts := embeddingModelListOptions{}
	cmd := &cobra.Command{
		Use:   "list",
		Short: "List registered summary models",
		RunE: func(cmd *cobra.Command, args []string) error {
			msg, err := listSummaryModelCmdFn(cmd.Context(), opts)
			if err != nil {
				return err
			}
			_, err = fmt.Fprintln(cmd.OutOrStdout(), msg)
			return err
		},
	}
	cmd.Flags().StringVar(&opts.DatabaseURL, "database-url", "", "PostgreSQL URL (overrides config file and POSTBRAIN_DATABASE_URL)")
	return cmd
}

func runRegisterEmbeddingModelCommand(ctx context.Context, opts embeddingModelRegisterOptions) (string, error) {
	return runRegisterModelCommand(ctx, opts, "embedding")
}

func runRegisterSummaryModelCommand(ctx context.Context, opts embeddingModelRegisterOptions) (string, error) {
	opts.ContentType = "text"
	if opts.Dimensions <= 0 {
		opts.Dimensions = 1
	}
	return runRegisterModelCommand(ctx, opts, "generation")
}

func runRegisterModelCommand(ctx context.Context, opts embeddingModelRegisterOptions, modelType string) (string, error) {
	embCfg, err := loadCLIEmbeddingConfig(cfgPath)
	if err != nil {
		return "", err
	}
	resolved, err := resolveProviderRegistrationFields(opts, embCfg, modelType)
	if err != nil {
		return "", err
	}

	pool, err := openCLIPool(ctx, opts.DatabaseURL)
	if err != nil {
		return "", err
	}
	defer pool.Close()

	model, err := db.RegisterEmbeddingModel(ctx, pool, db.RegisterEmbeddingModelParams{
		Slug:           opts.Slug,
		Provider:       resolved.Provider,
		ServiceURL:     resolved.ServiceURL,
		ProviderModel:  resolved.ProviderModel,
		ProviderConfig: resolved.ProviderConfig,
		Dimensions:     opts.Dimensions,
		ContentType:    opts.ContentType,
		ModelType:      modelType,
		Activate:       opts.Activate,
	})
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("registered model %s", model.Slug), nil
}

func loadCLIEmbeddingConfig(path string) (*config.EmbeddingConfig, error) {
	cfgPathVal := strings.TrimSpace(path)
	if cfgPathVal == "" {
		cfgPathVal = "config.yaml"
	}
	v := viper.New()
	v.SetConfigFile(cfgPathVal)
	v.SetEnvPrefix("POSTBRAIN")
	v.SetEnvKeyReplacer(strings.NewReplacer(".", "_"))
	v.AutomaticEnv()
	if err := v.ReadInConfig(); err != nil {
		return nil, fmt.Errorf("load config: %w", err)
	}

	var emb config.EmbeddingConfig
	if err := v.UnmarshalKey("embedding", &emb); err != nil {
		return nil, fmt.Errorf("load embedding config: %w", err)
	}
	if emb.Providers == nil {
		emb.Providers = map[string]config.EmbeddingProviderConfig{}
	}
	return &emb, nil
}

func resolveProviderRegistrationFields(opts embeddingModelRegisterOptions, embCfg *config.EmbeddingConfig, modelType string) (resolvedEmbeddingRegistration, error) {
	out := resolvedEmbeddingRegistration{}
	profileName := strings.TrimSpace(opts.ProviderConfig)
	if profileName == "" {
		profileName = "default"
	}
	out.ProviderConfig = profileName

	if modelType == "embedding" && opts.ContentType != "text" && opts.ContentType != "code" {
		return out, fmt.Errorf("invalid content type %q", opts.ContentType)
	}
	if modelType == "generation" && opts.ContentType != "text" {
		return out, fmt.Errorf("summary models require content type \"text\"")
	}
	if embCfg == nil {
		return out, fmt.Errorf("embedding config is not available")
	}
	profile, ok := embCfg.Providers[profileName]
	if !ok {
		return out, fmt.Errorf("embedding provider profile %q not found", profileName)
	}

	out.Provider = strings.TrimSpace(profile.Backend)
	out.ServiceURL = strings.TrimSpace(profile.ServiceURL)
	if modelType == "generation" {
		out.ProviderModel = strings.TrimSpace(profile.SummaryModel)
	} else {
		switch opts.ContentType {
		case "text":
			out.ProviderModel = strings.TrimSpace(profile.TextModel)
		case "code":
			out.ProviderModel = strings.TrimSpace(profile.CodeModel)
		}
	}
	if out.Provider == "" {
		return out, fmt.Errorf("embedding.providers.%s.backend is required", profileName)
	}
	if strings.EqualFold(out.Provider, "openai") && out.ServiceURL == "" {
		out.ServiceURL = "https://api.openai.com/v1"
	}
	if out.ServiceURL == "" {
		return out, fmt.Errorf("embedding.providers.%s.service_url is required", profileName)
	}
	if out.ProviderModel == "" && modelType == "generation" {
		return out, fmt.Errorf("embedding.providers.%s.summary_model is required", profileName)
	}
	if out.ProviderModel == "" && modelType == "embedding" {
		return out, fmt.Errorf("embedding.providers.%s.%s_model is required", profileName, opts.ContentType)
	}
	return out, nil
}

func runActivateEmbeddingModelCommand(ctx context.Context, opts embeddingModelActivateOptions) (string, error) {
	return runActivateModelCommand(ctx, opts, "embedding")
}

func runActivateSummaryModelCommand(ctx context.Context, opts embeddingModelActivateOptions) (string, error) {
	opts.ContentType = "text"
	return runActivateModelCommand(ctx, opts, "generation")
}

func runActivateModelCommand(ctx context.Context, opts embeddingModelActivateOptions, modelType string) (string, error) {
	if modelType == "embedding" && opts.ContentType != "text" && opts.ContentType != "code" {
		return "", fmt.Errorf("invalid content type %q", opts.ContentType)
	}
	if modelType == "generation" && opts.ContentType != "text" {
		return "", fmt.Errorf("summary models require content type \"text\"")
	}

	pool, err := openCLIPool(ctx, opts.DatabaseURL)
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
		UPDATE ai_models
		SET is_active = false
		WHERE content_type = $1 AND model_type = $2
	`, opts.ContentType, modelType); err != nil {
		return "", fmt.Errorf("deactivate models: %w", err)
	}

	tag, err := tx.Exec(ctx, `
		UPDATE ai_models
		SET is_active = true
		WHERE slug = $1 AND content_type = $2 AND model_type = $3
	`, opts.Slug, opts.ContentType, modelType)
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
	return runListModelsCommand(ctx, opts, "embedding")
}

func runListSummaryModelsCommand(ctx context.Context, opts embeddingModelListOptions) (string, error) {
	return runListModelsCommand(ctx, opts, "generation")
}

func runListModelsCommand(ctx context.Context, opts embeddingModelListOptions, modelType string) (string, error) {
	pool, err := openCLIPool(ctx, opts.DatabaseURL)
	if err != nil {
		return "", err
	}
	defer pool.Close()

	rows, err := pool.Query(ctx, `
		SELECT slug, provider, provider_model, content_type, dimensions, is_active, is_ready, COALESCE(table_name, '')
		FROM ai_models
		WHERE model_type = $1
		ORDER BY content_type, slug
	`, modelType)
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

func openCLIPool(ctx context.Context, databaseURL string) (*pgxpool.Pool, error) {
	url := strings.TrimSpace(databaseURL)
	if url == "" {
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
