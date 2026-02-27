package cmd

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/jescarri/go-joplin/internal/store"
	"github.com/jescarri/go-joplin/internal/sync"
	"github.com/spf13/cobra"
)

var syncCmd = &cobra.Command{
	Use:   "sync",
	Short: "Perform a one-shot sync with Joplin Server",
	RunE: func(cmd *cobra.Command, args []string) error {
		cfg, err := loadConfig()
		if err != nil {
			return err
		}

		db, err := store.Open(cfg.DataDir)
		if err != nil {
			return err
		}
		defer db.Close()

		var backend sync.SyncBackend
		if cfg.SyncTarget == 8 {
			backend, err = sync.NewS3Backend(cfg)
			if err != nil {
				return fmt.Errorf("S3 backend: %w", err)
			}
		} else {
			backend, err = sync.NewClient(cfg)
			if err != nil {
				return err
			}
		}

		engine := sync.NewEngine(backend, db, cfg.MasterPassword)

		slog.Info("starting sync")
		if err := engine.Sync(context.Background()); err != nil {
			return fmt.Errorf("sync failed: %w", err)
		}

		slog.Info("sync completed successfully")
		return nil
	},
}

func init() {
	rootCmd.AddCommand(syncCmd)
}
