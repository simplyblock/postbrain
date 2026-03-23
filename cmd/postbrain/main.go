// Command postbrain is the Postbrain server and migration runner.
package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"text/tabwriter"
	"time"

	"github.com/google/uuid"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/spf13/cobra"

	mcpapi "github.com/simplyblock/postbrain/internal/api/mcp"
	restapi "github.com/simplyblock/postbrain/internal/api/rest"
	"github.com/simplyblock/postbrain/internal/auth"
	"github.com/simplyblock/postbrain/internal/config"
	"github.com/simplyblock/postbrain/internal/db"
	"github.com/simplyblock/postbrain/internal/embedding"
	"github.com/simplyblock/postbrain/internal/jobs"
	uiapi "github.com/simplyblock/postbrain/internal/ui"
)

var cfgPath string

func main() {
	root := &cobra.Command{
		Use:   "postbrain",
		Short: "Postbrain — long-term memory for AI coding agents",
	}
	root.PersistentFlags().StringVar(&cfgPath, "config", "config.yaml", "path to config file")

	root.AddCommand(serveCmd(), migrateCmd(), tokenCmd(), onboardCmd())
	if err := root.Execute(); err != nil {
		os.Exit(1)
	}
}

func serveCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "serve",
		Short: "Start the Postbrain server",
		RunE:  runServe,
	}
}

func runServe(cmd *cobra.Command, args []string) error {
	ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer cancel()

	cfg, err := config.Load(cfgPath)
	if err != nil {
		return fmt.Errorf("load config: %w", err)
	}

	pool, err := db.NewPool(ctx, &cfg.Database)
	if err != nil {
		return fmt.Errorf("connect to database: %w", err)
	}
	defer pool.Close()

	if cfg.Database.AutoMigrate {
		if err := db.CheckAndMigrate(ctx, pool, true); err != nil {
			return fmt.Errorf("migrate: %w", err)
		}
	}

	svc, err := embedding.NewService(&cfg.Embedding)
	if err != nil {
		return fmt.Errorf("embedding service: %w", err)
	}

	// Build HTTP mux.
	mux := http.NewServeMux()

	// MCP server at /mcp.
	mcpSrv := mcpapi.NewServer(pool, svc, cfg)
	mux.Handle("/mcp", mcpSrv.Handler())
	mux.Handle("/mcp/", mcpSrv.Handler())

	// REST API.
	restSrv := restapi.NewRouter(pool, svc, cfg)
	mux.Handle("/", restSrv.Handler())

	// Web UI.
	uiHandler, err := uiapi.NewHandler(pool)
	if err != nil {
		return fmt.Errorf("ui handler: %w", err)
	}
	mux.Handle("/ui", uiHandler)
	mux.Handle("/ui/", uiHandler)

	// Prometheus metrics.
	mux.Handle("/metrics", promhttp.Handler())

	// HTTP server with graceful shutdown.
	httpSrv := &http.Server{
		Addr:         cfg.Server.Addr,
		Handler:      mux,
		ReadTimeout:  30 * time.Second,
		WriteTimeout: 60 * time.Second,
		IdleTimeout:  120 * time.Second,
	}

	ln, err := net.Listen("tcp", cfg.Server.Addr)
	if err != nil {
		return fmt.Errorf("listen on %s: %w", cfg.Server.Addr, err)
	}

	// Background jobs.
	scheduler := jobs.NewScheduler(pool, svc, &cfg.Jobs)
	if err := scheduler.Register(); err != nil {
		return fmt.Errorf("register jobs: %w", err)
	}
	scheduler.Start()
	defer scheduler.Stop(ctx)

	slog.Info("postbrain server starting", "addr", cfg.Server.Addr)

	errCh := make(chan error, 1)
	go func() {
		var serveErr error
		if cfg.Server.TLSCert != "" && cfg.Server.TLSKey != "" {
			serveErr = httpSrv.ServeTLS(ln, cfg.Server.TLSCert, cfg.Server.TLSKey)
		} else {
			serveErr = httpSrv.Serve(ln)
		}
		if !errors.Is(serveErr, http.ErrServerClosed) {
			errCh <- serveErr
		}
		close(errCh)
	}()

	select {
	case <-ctx.Done():
		slog.Info("shutting down server")
		shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer shutdownCancel()
		return httpSrv.Shutdown(shutdownCtx)
	case err := <-errCh:
		return err
	}
}

func migrateCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "migrate",
		Short: "Database migration management",
	}
	cmd.AddCommand(
		migrateUpCmd(),
		migrateDownCmd(),
		migrateStatusCmd(),
		migrateVersionCmd(),
		migrateForceCmd(),
	)
	return cmd
}

func migrateUpCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "up",
		Short: "Apply all pending migrations",
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := config.Load(cfgPath)
			if err != nil {
				return err
			}
			ctx := context.Background()
			pool, err := db.NewPool(ctx, &cfg.Database)
			if err != nil {
				return err
			}
			defer pool.Close()
			return db.CheckAndMigrate(ctx, pool, true)
		},
	}
}

func migrateDownCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "down [N]",
		Short: "Roll back N migrations (default 1)",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			// TODO(task-migrate): implement MigrateDown(N) in db package
			slog.Info("migrate down: not yet implemented")
			return nil
		},
	}
}

func migrateStatusCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "status",
		Short: "Show applied and pending migrations",
		RunE: func(cmd *cobra.Command, args []string) error {
			// TODO(task-migrate): implement MigrateStatus() in db package
			slog.Info("migrate status: not yet implemented")
			return nil
		},
	}
}

func migrateVersionCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "version",
		Short: "Print current schema version",
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := config.Load(cfgPath)
			if err != nil {
				return err
			}
			ctx := context.Background()
			pool, err := db.NewPool(ctx, &cfg.Database)
			if err != nil {
				return err
			}
			defer pool.Close()
			_ = pool // TODO(task-migrate): query schema version
			fmt.Println("schema version: (implement db.CurrentVersion)")
			return nil
		},
	}
}

func migrateForceCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "force [N]",
		Short: "Force schema version to N (clears dirty state)",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			// TODO(task-migrate): implement db.MigrateForce(N)
			slog.Info("migrate force: not yet implemented", "version", args[0])
			return nil
		},
	}
}

// ── token subcommand ──────────────────────────────────────────────────────────

func tokenCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "token",
		Short: "Manage API tokens",
	}
	cmd.AddCommand(tokenCreateCmd(), tokenListCmd(), tokenRevokeCmd())
	return cmd
}

func tokenCreateCmd() *cobra.Command {
	var (
		principalSlug string
		name          string
		permissions   []string
	)
	cmd := &cobra.Command{
		Use:   "create",
		Short: "Create a new API token and print it once",
		RunE: func(cmd *cobra.Command, args []string) error {
			if principalSlug == "" {
				return fmt.Errorf("--principal is required")
			}
			if name == "" {
				return fmt.Errorf("--name is required")
			}
			cfg, err := config.Load(cfgPath)
			if err != nil {
				return err
			}
			ctx := context.Background()
			pool, err := db.NewPool(ctx, &cfg.Database)
			if err != nil {
				return err
			}
			defer pool.Close()

			principal, err := db.GetPrincipalBySlug(ctx, pool, principalSlug)
			if err != nil {
				return fmt.Errorf("lookup principal: %w", err)
			}
			if principal == nil {
				return fmt.Errorf("principal %q not found", principalSlug)
			}

			raw, hash, err := auth.GenerateToken()
			if err != nil {
				return err
			}

			if len(permissions) == 0 {
				permissions = []string{"read", "write"}
			}

			t, err := db.CreateToken(ctx, pool, principal.ID, hash, name, nil, permissions, nil)
			if err != nil {
				return fmt.Errorf("create token: %w", err)
			}

			fmt.Printf("Token ID:    %s\n", t.ID)
			fmt.Printf("Principal:   %s (%s)\n", principal.Slug, principal.ID)
			fmt.Printf("Name:        %s\n", t.Name)
			fmt.Printf("Permissions: %s\n", strings.Join(t.Permissions, ", "))
			fmt.Printf("Created:     %s\n", t.CreatedAt.Format(time.RFC3339))
			fmt.Printf("\nToken (shown once — store it now):\n\n  %s\n\n", raw)
			return nil
		},
	}
	cmd.Flags().StringVar(&principalSlug, "principal", "", "Principal slug to own the token (required)")
	cmd.Flags().StringVar(&name, "name", "", "Human-readable token name (required)")
	cmd.Flags().StringSliceVar(&permissions, "permissions", nil, "Comma-separated permissions (default: read,write)")
	return cmd
}

func tokenListCmd() *cobra.Command {
	var principalSlug string
	cmd := &cobra.Command{
		Use:   "list",
		Short: "List API tokens",
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := config.Load(cfgPath)
			if err != nil {
				return err
			}
			ctx := context.Background()
			pool, err := db.NewPool(ctx, &cfg.Database)
			if err != nil {
				return err
			}
			defer pool.Close()

			var tokens []*db.Token
			if principalSlug != "" {
				principal, err := db.GetPrincipalBySlug(ctx, pool, principalSlug)
				if err != nil {
					return fmt.Errorf("lookup principal: %w", err)
				}
				if principal == nil {
					return fmt.Errorf("principal %q not found", principalSlug)
				}
				tokens, err = db.ListTokens(ctx, pool, &principal.ID)
				if err != nil {
					return fmt.Errorf("list tokens: %w", err)
				}
			} else {
				tokens, err = db.ListTokens(ctx, pool, nil)
				if err != nil {
					return fmt.Errorf("list tokens: %w", err)
				}
			}

			if len(tokens) == 0 {
				fmt.Println("No tokens found.")
				return nil
			}

			w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
			fmt.Fprintln(w, "ID\tNAME\tPRINCIPAL\tPERMISSIONS\tCREATED\tREVOKED")
			for _, t := range tokens {
				revoked := ""
				if !t.RevokedAt.IsZero() {
					revoked = t.RevokedAt.Format(time.RFC3339)
				}
				fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\t%s\n",
					t.ID,
					t.Name,
					t.PrincipalID,
					strings.Join(t.Permissions, ","),
					t.CreatedAt.Format(time.RFC3339),
					revoked,
				)
			}
			return w.Flush()
		},
	}
	cmd.Flags().StringVar(&principalSlug, "principal", "", "Filter to tokens owned by this principal slug")
	return cmd
}

func tokenRevokeCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "revoke <token-id>",
		Short: "Revoke an API token by ID",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := config.Load(cfgPath)
			if err != nil {
				return err
			}
			ctx := context.Background()
			pool, err := db.NewPool(ctx, &cfg.Database)
			if err != nil {
				return err
			}
			defer pool.Close()

			tid, err := uuid.Parse(args[0])
			if err != nil {
				return fmt.Errorf("invalid token ID: %w", err)
			}

			if err := db.RevokeToken(ctx, pool, tid); err != nil {
				return fmt.Errorf("revoke token: %w", err)
			}
			fmt.Printf("Token %s revoked.\n", tid)
			return nil
		},
	}
}

// ── onboard subcommand ────────────────────────────────────────────────────────

func onboardCmd() *cobra.Command {
	var (
		databaseURL   string
		adminSlug     string
		displayName   string
		tokenName     string
		skipMigrate   bool
		textModel     string
		codeModel     string
		modelDims     int
	)

	cmd := &cobra.Command{
		Use:   "onboard",
		Short: "Bootstrap a fresh Postbrain installation",
		Long: `onboard sets up everything needed for a first run:

  1. Runs all pending migrations (unless --skip-migrate)
  2. Creates the admin principal (idempotent — reuses existing slug)
  3. Creates the admin's personal scope  (idempotent)
  4. Registers the configured embedding model in the DB (when --dimensions is given)
  5. Creates a new API token with full permissions
  6. Prints the raw token — store it now, it will not be shown again

Requires only a database URL, either via --database-url or the
POSTBRAIN_DATABASE_URL env var (or a config file with database.url set).`,
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx := context.Background()

			// ── 1. Resolve database URL ──────────────────────────────────────
			dbURL := databaseURL
			if dbURL == "" {
				var err error
				dbURL, err = config.LoadDatabaseURL(cfgPath)
				if err != nil {
					return fmt.Errorf("database URL not found: %w\n\nPass --database-url or set POSTBRAIN_DATABASE_URL", err)
				}
			}

			dbCfg := &config.DatabaseConfig{
				URL:            dbURL,
				MaxOpen:        5,
				MaxIdle:        2,
				ConnectTimeout: 15 * time.Second,
			}

			pool, err := db.NewPool(ctx, dbCfg)
			if err != nil {
				return fmt.Errorf("connect to database: %w", err)
			}
			defer pool.Close()

			fmt.Println("Postbrain — first-run setup")
			fmt.Println(strings.Repeat("─", 50))

			// ── 2. Migrations ────────────────────────────────────────────────
			if !skipMigrate {
				fmt.Print("Running migrations ... ")
				if err := db.CheckAndMigrate(ctx, pool, true); err != nil {
					return fmt.Errorf("migrate: %w", err)
				}
				fmt.Println("done")
			} else {
				fmt.Println("Skipping migrations (--skip-migrate)")
			}

			// ── 3. Admin principal ───────────────────────────────────────────
			fmt.Printf("Creating principal %q ... ", adminSlug)
			principal, err := db.GetPrincipalBySlug(ctx, pool, adminSlug)
			if err != nil {
				return fmt.Errorf("look up principal: %w", err)
			}
			if principal != nil {
				fmt.Printf("already exists (%s)\n", principal.ID)
			} else {
				principal, err = db.CreatePrincipal(ctx, pool, "user", adminSlug, displayName, nil)
				if err != nil {
					return fmt.Errorf("create principal: %w", err)
				}
				fmt.Printf("created (%s)\n", principal.ID)
			}

			// ── 4. Personal scope ────────────────────────────────────────────
			scopeExternalID := adminSlug
			fmt.Printf("Creating scope user:%s ... ", scopeExternalID)
			scope, err := db.GetScopeByExternalID(ctx, pool, "user", scopeExternalID)
			if err != nil {
				return fmt.Errorf("look up scope: %w", err)
			}
			if scope != nil {
				fmt.Printf("already exists (%s)\n", scope.ID)
			} else {
				scope, err = db.CreateScope(ctx, pool, "user", scopeExternalID, displayName, nil, principal.ID, nil)
				if err != nil {
					return fmt.Errorf("create scope: %w", err)
				}
				fmt.Printf("created (%s)\n", scope.ID)
			}

			// ── 5. Embedding model ───────────────────────────────────────────
			if modelDims > 0 {
				fmt.Printf("Registering text embedding model %q (%d dims) ... ", textModel, modelDims)
				q := db.New(pool)
				_, err := q.UpsertEmbeddingModel(ctx, db.UpsertEmbeddingModelParams{
					Slug:        textModel,
					Dimensions:  int32(modelDims), //nolint:gosec
					ContentType: "text",
					IsActive:    true,
				})
				if err != nil {
					return fmt.Errorf("register text model: %w", err)
				}
				fmt.Println("done")

				if codeModel != "" && codeModel != textModel {
					fmt.Printf("Registering code embedding model %q (%d dims) ... ", codeModel, modelDims)
					_, err = q.UpsertEmbeddingModel(ctx, db.UpsertEmbeddingModelParams{
						Slug:        codeModel,
						Dimensions:  int32(modelDims), //nolint:gosec
						ContentType: "code",
						IsActive:    true,
					})
					if err != nil {
						return fmt.Errorf("register code model: %w", err)
					}
					fmt.Println("done")
				}
			} else {
				fmt.Println("Skipping embedding model registration (pass --dimensions to register)")
			}

			// ── 6. API token ─────────────────────────────────────────────────
			fmt.Printf("Creating API token %q ... ", tokenName)
			raw, hash, err := auth.GenerateToken()
			if err != nil {
				return err
			}
			token, err := db.CreateToken(ctx, pool, principal.ID, hash, tokenName, nil,
				[]string{"read", "write", "admin"}, nil)
			if err != nil {
				return fmt.Errorf("create token: %w", err)
			}
			fmt.Printf("created (%s)\n", token.ID)

			// ── Summary ──────────────────────────────────────────────────────
			fmt.Println()
			fmt.Println(strings.Repeat("─", 50))
			fmt.Println("Setup complete. Summary:")
			fmt.Println()
			fmt.Printf("  Principal : %s  (kind=user, id=%s)\n", principal.Slug, principal.ID)
			fmt.Printf("  Scope     : user:%s  (id=%s)\n", scope.ExternalID, scope.ID)
			fmt.Printf("  Token ID  : %s\n", token.ID)
			fmt.Printf("  Token name: %s\n", token.Name)
			fmt.Println()
			fmt.Println("  Bearer token (shown once — store it now):")
			fmt.Println()
			fmt.Printf("    %s\n", raw)
			fmt.Println()
			fmt.Println("  Use it in:")
			fmt.Printf("    curl -H 'Authorization: Bearer %s' http://localhost:7433/v1/memories/recall?q=hello&scope=user:%s\n", raw, adminSlug)
			fmt.Println()
			fmt.Println("  For the Web UI, open http://localhost:7433/ui and paste the token.")
			fmt.Println()
			return nil
		},
	}

	cmd.Flags().StringVar(&databaseURL, "database-url", "", "PostgreSQL URL (overrides config file and POSTBRAIN_DATABASE_URL)")
	cmd.Flags().StringVar(&adminSlug, "slug", "admin", "Slug for the admin principal")
	cmd.Flags().StringVar(&displayName, "display-name", "Admin", "Display name for the admin principal")
	cmd.Flags().StringVar(&tokenName, "token-name", "admin", "Name for the generated API token")
	cmd.Flags().BoolVar(&skipMigrate, "skip-migrate", false, "Skip running database migrations")
	cmd.Flags().StringVar(&textModel, "text-model", "nomic-embed-text", "Text embedding model slug to register")
	cmd.Flags().StringVar(&codeModel, "code-model", "", "Code embedding model slug to register (optional)")
	cmd.Flags().IntVar(&modelDims, "dimensions", 0, "Embedding vector dimensions; required to register the model (e.g. 768 for nomic-embed-text)")

	return cmd
}
