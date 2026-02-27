package cmd

import (
	"fmt"
	"strings"

	"github.com/spf13/cobra"
)

var configCmd = &cobra.Command{
	Use:   "config",
	Short: "Print resolved configuration",
	RunE: func(cmd *cobra.Command, args []string) error {
		cfg, err := loadConfig()
		if err != nil {
			return err
		}

		fmt.Printf("Sync Target:    %d\n", cfg.SyncTarget)
		fmt.Printf("Server URL:     %s\n", cfg.ServerURL)
		fmt.Printf("Username:       %s\n", cfg.Username)
		fmt.Printf("Password:       %s\n", redact(cfg.Password))
		fmt.Printf("API Token:      %s\n", redact(cfg.APIToken))
		fmt.Printf("API Key:        %s\n", redact(cfg.APIKey))
		fmt.Printf("Master Pwd:     %s\n", redact(cfg.MasterPassword))
		fmt.Printf("Data Dir:       %s\n", cfg.DataDir)
		fmt.Printf("Listen Address: %s\n", cfg.ListenAddr())
		return nil
	},
}

func redact(s string) string {
	if s == "" {
		return "(not set)"
	}
	if len(s) <= 4 {
		return strings.Repeat("*", len(s))
	}
	return s[:2] + strings.Repeat("*", len(s)-4) + s[len(s)-2:]
}

func init() {
	rootCmd.AddCommand(configCmd)
}
