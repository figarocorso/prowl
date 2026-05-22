package cmd

import (
	"fmt"
	"time"

	"github.com/figarocorso/prowl/internal/tui"
	"github.com/spf13/cobra"
)

// watchMinInterval is the smallest auto-refresh cadence we accept. Anything
// lower would risk hammering GitHub (and, for queued PRs, the merge-queue
// endpoint) faster than the data meaningfully changes.
const watchMinInterval = 5 * time.Second

// watchInterval defaults to 30s — long enough to be polite to the GitHub API
// (and merge-queue lookups), short enough that a PR state change shows up
// before the user gives up on the screen.
var watchInterval time.Duration

var watchCmd = &cobra.Command{
	Use:   "watch",
	Short: "Open the TUI and auto-refresh every --interval",
	Long: `watch opens the same interactive TUI as 'prowl' but periodically
re-fetches the active PR list (default every 30s). Press q or Ctrl-C to exit.`,
	RunE: runWatch,
}

func init() {
	watchCmd.Flags().DurationVar(&watchInterval, "interval", 30*time.Second, "auto-refresh interval (minimum 5s)")
	rootCmd.AddCommand(watchCmd)
}

func runWatch(_ *cobra.Command, _ []string) error {
	if watchInterval < watchMinInterval {
		return fmt.Errorf("--interval must be at least %s (got %s)", watchMinInterval, watchInterval)
	}
	return tui.RunWatch(dataDir, watchInterval)
}
