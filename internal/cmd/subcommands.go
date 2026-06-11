package cmd

import (
	"fmt"
	"os"
	"time"

	"github.com/spf13/cobra"
	"installr/internal/scanner"
	"installr/internal/store"
)

var scanCmd = &cobra.Command{
	Use:   "scan",
	Short: "Rescan all package managers and print summary",
	RunE: func(cmd *cobra.Command, args []string) error {
		s, err := store.Open(dbPath)
		if err != nil {
			return err
		}
		defer s.Close()

		scanners := scanner.DiscoverScanners()

		cutoff := time.Now()
		for _, sc := range scanners {
			pkgs, err := sc.Scan()
			if err != nil {
				fmt.Fprintf(os.Stderr, "warn: %s scan failed: %v\n", sc.Name(), err)
				continue
			}
			for _, p := range pkgs {
				_ = s.Upsert(p)
			}
		}

		_ = s.PurgeStale(cutoff)

		counts, total, err := s.CountBySource(false)
		if err != nil {
			return err
		}

		fmt.Printf("Total packages: %d\n", total)
		for _, sc := range scanner.AllScanners {
			if c := counts[sc.Name()]; c > 0 {
				fmt.Printf("  %s: %d\n", sc.Name(), c)
			}
		}
		return nil
	},
}

func init() {
	rootCmd.AddCommand(scanCmd)
}
