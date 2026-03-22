package cmd

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/jescarri/go-joplin/internal/clipper"
	"github.com/jescarri/go-joplin/internal/mcp"
	"github.com/jescarri/go-joplin/internal/store"
	"github.com/jescarri/go-joplin/internal/sync"
	"github.com/jescarri/go-joplin/internal/telemetry"
	"github.com/spf13/cobra"
)

var serveCmd = &cobra.Command{
	Use:   "serve",
	Short: "Start the clipper server with background sync",
	RunE: func(cmd *cobra.Command, args []string) error {
		cfg, err := loadConfig()
		if err != nil {
			return err
		}
		if cfg.APIToken == "" {
			return fmt.Errorf("api token is required: set api.token in config (e.g. ${GOJOPLIN_API_TOKEN}) or GOJOPLIN_API_TOKEN env var")
		}
		if cfg.APIKey == "" {
			return fmt.Errorf("api key is required: set api.key in config (e.g. ${GOJOPLIN_API_KEY}) or GOJOPLIN_API_KEY env var")
		}

		ctx := context.Background()
		if cfg.Observability.Tracing.Enabled {
			shutdown, err := telemetry.InitTracing(ctx, cfg.Observability.Tracing)
			if err != nil {
				return fmt.Errorf("tracing: %w", err)
			}
			if shutdown != nil {
				defer func() { _ = shutdown(context.Background()) }()
			}
		}

		if cfg.Observability.Metrics.Enabled && cfg.Observability.Metrics.PrometheusPort > 0 {
			metricsAddr := fmt.Sprintf(":%d", cfg.Observability.Metrics.PrometheusPort)
			go func() {
				if err := telemetry.StartMetricsServer(ctx, metricsAddr, func() {
					slog.Info("metrics server listening", "addr", metricsAddr)
				}); err != nil {
					slog.Error("metrics server failed", "error", err)
				}
			}()
		}

		db, err := store.Open(cfg.DataDir)
		if err != nil {
			return err
		}
		defer db.Close()

		var backend sync.SyncBackend
		if cfg.SyncTarget == 8 {
			b, err := sync.NewS3Backend(cfg)
			if err != nil {
				return fmt.Errorf("S3 backend: %w", err)
			}
			backend = sync.NewTracedBackend(b, "s3")
		} else {
			c, err := sync.NewClient(cfg)
			if err != nil {
				return err
			}
			backend = sync.NewTracedBackend(c, "joplin_server")
		}

		engine := sync.NewEngine(backend, db, cfg.MasterPassword)

		// Background sync loop
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		go func() {
			ticker := time.NewTicker(10 * time.Minute)
			defer ticker.Stop()

			// Initial sync
			if err := engine.Sync(ctx); err != nil {
				slog.Error("initial sync failed", "error", err)
			}

			for {
				select {
				case <-ctx.Done():
					return
				case <-ticker.C:
					if err := engine.Sync(ctx); err != nil {
						slog.Error("sync failed", "error", err)
					}
				case <-engine.TriggerCh():
					if err := engine.Sync(ctx); err != nil {
						slog.Error("triggered sync failed", "error", err)
					}
				}
			}
		}()

		policy := mcp.NewPolicy(cfg)
		var mcpHandler http.Handler
		{
			mcpDeps := &mcp.Deps{DB: db, Syncer: engine, Policy: policy, EnabledTools: cfg.MCPEnabledTools}
			mcpServer := mcp.NewServer(mcpDeps)
			mcpHandler = mcp.NewSSEHandler(func(r *http.Request) *mcp.Server { return mcpServer })
		}
		srv := clipper.NewServer(db, cfg.APIToken, cfg.APIKey, engine, policy, mcpHandler)
		addr := cfg.ListenAddr()
		slog.Info("starting clipper server", "addr", addr)

		httpSrv := &http.Server{
			Addr:    addr,
			Handler: srv.Router(),
		}

		// Graceful shutdown
		sigCh := make(chan os.Signal, 1)
		signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)

		go func() {
			<-sigCh
			slog.Info("shutting down")
			cancel()
			shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer shutdownCancel()
			httpSrv.Shutdown(shutdownCtx)
		}()

		if err := httpSrv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			return err
		}
		return nil
	},
}

func init() {
	rootCmd.AddCommand(serveCmd)
}
