package cmd

import (
	"context"
	"fmt"

	"github.com/figarocorso/prowl/internal/data"
	"github.com/figarocorso/prowl/internal/ui"
	"github.com/spf13/cobra"
)

var archiveCmd = &cobra.Command{
	Use:     "archive",
	Aliases: []string{"clean"},
	Short:   "Move merged or closed PRs from active to reviewed",
	RunE:    runArchive,
}

func init() {
	rootCmd.AddCommand(archiveCmd)
}

func runArchive(cmd *cobra.Command, _ []string) error {
	_, store, err := loadConfigAndStore()
	if err != nil {
		return err
	}
	urls, err := store.Active()
	if err != nil {
		return err
	}
	if len(urls) == 0 {
		fmt.Fprintln(cmd.OutOrStderr(), "no active PRs tracked")
		return nil
	}
	client, err := clientFactory()
	if err != nil {
		return err
	}
	results := client.FetchBatch(context.Background(), urls)

	plainErr := ui.IsPlain(cmd.OutOrStderr())
	plainOut := ui.IsPlain(cmd.OutOrStdout())
	var done []string
	for _, r := range results {
		if r.Err != nil {
			fmt.Fprintf(cmd.OutOrStderr(), "%s fetch %s: %v\n", ui.Warn(plainErr), r.URL, r.Err)
			continue
		}
		if data.IsTerminal(r.PR) {
			done = append(done, r.URL)
		}
	}
	if len(done) == 0 {
		fmt.Fprintf(cmd.OutOrStdout(), "%s nothing to archive\n", ui.OK(plainOut))
		return nil
	}
	moved, err := store.MoveActiveToReviewed(done)
	if err != nil {
		return err
	}
	fmt.Fprintf(cmd.OutOrStdout(), "%s archived %d PR(s)\n", ui.OK(plainOut), moved)
	for _, u := range done {
		fmt.Fprintf(cmd.OutOrStdout(), "  %s %s\n", ui.Arrow(plainOut), u)
	}
	return nil
}
