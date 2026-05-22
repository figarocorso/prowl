package cmd

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/figarocorso/prowl/internal/data"
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
	fmt.Fprintf(out, "PR #%d — %s\n", pr.Number, pr.Title)
	fmt.Fprintf(out, "  URL:       %s\n", pr.URL)
	fmt.Fprintf(out, "  State:     %s\n", data.StatusLabel(pr))
	fmt.Fprintf(out, "  Assignees: %s\n", data.AssigneesLabel(pr))
	fmt.Fprintf(out, "  Queue:     %s (pos %s, ETA %s)\n",
		data.QueueLabel(pr), data.QueuePositionLabel(pr), data.ETALabel(pr))
	return nil
}
