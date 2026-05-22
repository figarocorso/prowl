package cmd

import (
	"fmt"

	"github.com/figarocorso/prowl/internal/data"
	"github.com/figarocorso/prowl/internal/ui"
	"github.com/spf13/cobra"
)

var removeCmd = &cobra.Command{
	Use:     "remove URL [URL...]",
	Aliases: []string{"rm", "delete"},
	Short:   "Stop tracking a PR (remove from both lists)",
	Args:    cobra.MinimumNArgs(1),
	RunE:    runRemove,
}

func init() {
	rootCmd.AddCommand(removeCmd)
}

func runRemove(cmd *cobra.Command, args []string) error {
	_, store, err := loadConfigAndStore()
	if err != nil {
		return err
	}
	plainErr := ui.IsPlain(cmd.OutOrStderr())
	plainOut := ui.IsPlain(cmd.OutOrStdout())
	totalRemoved := 0
	for _, raw := range args {
		url, err := data.CanonicalURL(raw)
		if err != nil {
			fmt.Fprintf(cmd.OutOrStderr(), "%s %s\n", ui.Err(plainErr), err)
			continue
		}
		n, err := store.Remove(url)
		if err != nil {
			return err
		}
		if n == 0 {
			fmt.Fprintf(cmd.OutOrStderr(), "%s not tracked: %s\n", ui.Warn(plainErr), url)
			continue
		}
		fmt.Fprintf(cmd.OutOrStdout(), "%s removed: %s (%d row(s))\n", ui.OK(plainOut), url, n)
		totalRemoved += n
	}
	if totalRemoved == 0 {
		return fmt.Errorf("no PRs removed")
	}
	return nil
}
