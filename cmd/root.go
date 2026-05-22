package cmd

import (
	"fmt"
	"os"

	"github.com/figarocorso/prowl/internal/ui"
	"github.com/spf13/cobra"
)

var (
	dataDir string
	profile string
	jsonOut bool
)

var rootCmd = &cobra.Command{
	Use:   "prowl",
	Short: "🦉 Keep watch over your pull requests",
	Long: `prowl — track GitHub Pull Requests you care about.

Run with no arguments to open the interactive TUI. Use subcommands for
non-interactive workflows (add, list, remove, archive, check).`,
	SilenceUsage: true,
	RunE: func(cmd *cobra.Command, args []string) error {
		return runTUI()
	},
}

func Execute() {
	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func init() {
	rootCmd.PersistentFlags().StringVar(&dataDir, "data-dir", "", "directory holding list files (default: $XDG_DATA_HOME/prowl)")
	rootCmd.PersistentFlags().StringVar(&profile, "profile", "", "profile subdir of the data dir, e.g. work or personal (also honors PROWL_PROFILE)")
	rootCmd.PersistentFlags().BoolVar(&ui.Plain, "plain", false, "disable colors/emoji; emit ASCII-only output (also honors NO_COLOR)")
	rootCmd.PersistentFlags().BoolVar(&ui.Plain, "no-color", false, "alias for --plain")
}
