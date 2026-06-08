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

		scanners := []scanner.Scanner{
			scanner.AptScanner{},
			scanner.SnapScanner{},
			scanner.NpmScanner{},
			scanner.PipScanner{},
			scanner.CondaScanner{},
			scanner.BinScanner{},
		}

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

		counts, total, err := s.CountBySource()
		if err != nil {
			return err
		}

		fmt.Printf("Total packages: %d\n", total)
		for _, src := range []string{"apt", "snap", "npm", "pip", "conda", "bin"} {
			fmt.Printf("  %s: %d\n", src, counts[src])
		}
		return nil
	},
}

var uninstallCmd = &cobra.Command{
	Use:   "uninstall <name>",
	Short: "Uninstall a package",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		source, _ := cmd.Flags().GetString("source")
		location, _ := cmd.Flags().GetString("location")
		if source == "" {
			return fmt.Errorf("--source is required (apt, snap, npm, pip, conda, bin)")
		}
		if location == "" {
			location = "system"
		}

		var sc scanner.Scanner
		switch source {
		case "apt":
			sc = scanner.AptScanner{}
		case "snap":
			sc = scanner.SnapScanner{}
		case "npm":
			sc = scanner.NpmScanner{}
		case "pip":
			sc = scanner.PipScanner{}
		case "conda":
			sc = scanner.CondaScanner{}
		case "bin":
			sc = scanner.BinScanner{}
		default:
			return fmt.Errorf("unknown source: %s", source)
		}

		fmt.Printf("Uninstalling %s (%s) from %s...\n", args[0], source, location)
		return sc.Uninstall(args[0], location)
	},
}

var installCmd = &cobra.Command{
	Use:   "install <name>",
	Short: "Install a package",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		source, _ := cmd.Flags().GetString("source")
		location, _ := cmd.Flags().GetString("location")
		if source == "" {
			return fmt.Errorf("--source is required (apt, snap, npm, pip, conda, bin)")
		}
		if location == "" {
			location = "system"
		}

		var sc scanner.Scanner
		switch source {
		case "apt":
			sc = scanner.AptScanner{}
		case "snap":
			sc = scanner.SnapScanner{}
		case "npm":
			sc = scanner.NpmScanner{}
		case "pip":
			sc = scanner.PipScanner{}
		case "conda":
			sc = scanner.CondaScanner{}
		case "bin":
			sc = scanner.BinScanner{}
		default:
			return fmt.Errorf("unknown source: %s", source)
		}

		fmt.Printf("Installing %s (%s) to %s...\n", args[0], source, location)
		return sc.Install(args[0], location)
	},
}

func init() {
	rootCmd.AddCommand(scanCmd)
	rootCmd.AddCommand(uninstallCmd)
	rootCmd.AddCommand(installCmd)
	uninstallCmd.Flags().String("source", "", "Package source (apt, snap, npm, pip, conda, bin)")
	uninstallCmd.Flags().String("location", "system", "Package location (system or path)")
	installCmd.Flags().String("source", "", "Package source (apt, snap, npm, pip, conda, bin)")
	installCmd.Flags().String("location", "system", "Package location (system or path)")
}
