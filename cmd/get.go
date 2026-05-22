package cmd

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/figarocorso/prowl/internal/data"
	"github.com/figarocorso/prowl/internal/ui"
	"github.com/spf13/cobra"
)

var getCmd = &cobra.Command{
	Use:   "get URL",
	Short: "Fetch a single PR (use --json for agent-friendly output)",
	Args:  cobra.ExactArgs(1),
	RunE:  runGet,
}

func init() {
	getCmd.Flags().BoolVar(&jsonOut, "json", false, "emit JSON instead of human-readable output")
	rootCmd.AddCommand(getCmd)
}

func runGet(cmd *cobra.Command, args []string) error {
	url, err := data.CanonicalURL(args[0])
	if err != nil {
		return err
	}
	client, err := clientFactory()
	if err != nil {
		return err
	}
	pr, err := client.Fetch(context.Background(), url)
	if err != nil {
		return err
	}
	if jsonOut {
		enc := json.NewEncoder(cmd.OutOrStdout())
		enc.SetIndent("", "  ")
		return enc.Encode(pr)
	}
	out := cmd.OutOrStdout()
	plain := ui.IsPlain(out)
	fmt.Fprintf(out, "%s %s\n",
		ui.Title(plain, fmt.Sprintf("PR #%d", pr.Number)),
		pr.Title,
	)
	label := func(s string) string { return ui.Dim(plain, s) }
	fmt.Fprintf(out, "  %s %s\n", label("URL:      "), ui.Dim(plain, pr.URL))
	fmt.Fprintf(out, "  %s %s\n", label("State:    "), ui.StatusBadge(plain, data.StatusLabel(pr)))
	fmt.Fprintf(out, "  %s %s\n", label("Assignees:"), data.AssigneesLabel(pr))
	fmt.Fprintf(out, "  %s %s (pos %s, ETA %s)\n",
		label("Queue:    "),
		data.QueueLabel(pr), data.QueuePositionLabel(pr), data.ETALabel(pr))
	return nil
}
