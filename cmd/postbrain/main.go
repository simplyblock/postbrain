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
	"syscall"
	"time"

	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/spf13/cobra"

	mcpapi "github.com/simplyblock/postbrain/internal/api/mcp"
	restapi "github.com/simplyblock/postbrain/internal/api/rest"
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

	root.AddCommand(serveCmd(), migrateCmd())
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
