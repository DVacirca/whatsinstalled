package cmd

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
	"whatsinstalled/internal/store"
	"whatsinstalled/internal/tui"
	"whatsinstalled/internal/version"
)

var dbPath string

func init() {
	rootCmd.PersistentFlags().StringVar(&dbPath, "db", store.DBPath(), "Path to SQLite database")
}

var rootCmd = &cobra.Command{
	Use:     "whatsinstalled",
	Version: version.Version,
	Short:   "A TUI dashboard for system packages",
	Long:    `whatsinstalled gives you an overview of packages installed via apt, snap, npm, and pip.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		return runDashboard()
	},
}

// Execute runs the root command.
func Execute() {
	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func runDashboard() error {
	s, err := store.Open(dbPath)
	if err != nil {
		return err
	}
	defer s.Close()
	return tui.Run(s)
}
