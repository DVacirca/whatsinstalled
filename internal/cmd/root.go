package cmd

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
	"installr/internal/store"
	"installr/internal/tui"
)

var dbPath string

func init() {
	rootCmd.PersistentFlags().StringVar(&dbPath, "db", store.DBPath(), "Path to SQLite database")
}

var rootCmd = &cobra.Command{
	Use:   "installr",
	Short: "A TUI dashboard for system packages",
	Long:  `installr gives you an overview of packages installed via apt, snap, npm, and pip.`,
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
