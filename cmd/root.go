package cmd

import (
	"fmt"
	"os"

	"github.com/jescarri/go-joplin/internal/config"
	"github.com/spf13/cobra"
)

var (
	cfgFile        string
	username       string
	password       string
	apiKey         string
	masterPassword string
)

var rootCmd = &cobra.Command{
	Use:   "joplingo",
	Short: "Headless Joplin clipper server",
	Long:  "A headless Joplin clipper server that syncs with Joplin Server and exposes the Clipper REST API.",
}

func loadConfig() (*config.Config, error) {
	return config.Load(cfgFile, config.Overrides{
		Username:       username,
		Password:       password,
		APIKey:         apiKey,
		MasterPassword: masterPassword,
	})
}

func Execute() {
	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func init() {
	rootCmd.PersistentFlags().StringVar(&cfgFile, "config", "", "config file path (default: ~/.config/joplin-desktop/settings.json)")
	rootCmd.PersistentFlags().StringVar(&username, "username", "", "Joplin Server username (env: GOJOPLIN_USERNAME)")
	rootCmd.PersistentFlags().StringVar(&password, "password", "", "Joplin Server password (env: GOJOPLIN_PASSWORD)")
	rootCmd.PersistentFlags().StringVar(&apiKey, "api-key", "", "API key for clipper server authentication (env: GOJOPLIN_API_KEY)")
	rootCmd.PersistentFlags().StringVar(&masterPassword, "master-password", "", "E2EE master password for decrypting notes (env: GOJOPLIN_MASTER_PASSWORD)")
}
