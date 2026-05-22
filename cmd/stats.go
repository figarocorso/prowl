package cmd

import (
	"context"
	"encoding/json"
	"fmt"
	"io"

	"github.com/figarocorso/prowl/internal/data"
	"github.com/figarocorso/prowl/internal/ui"
	"github.com/spf13/cobra"
)

var statsCmd = &cobra.Command{
	Use:     "stats",
	Aliases: []string{"usage"},
	Short:   "Show counts of PRs prowl has helped manage",
	RunE:    runStats,
}

func init() {
	statsCmd.Flags().BoolVar(&jsonOut, "json", false, "emit JSON instead of human-readable output")
	rootCmd.AddCommand(statsCmd)
}

func runStats(cmd *cobra.Command, _ []string) error {
	_, s, err := loadConfigAndStore()
	if err != nil {
		return err
	}
	active, err := s.Active()
	if err != nil {
		return err
	}
	reviewed, err := s.Reviewed()
	if err != nil {
		return err
	}
	var results []data.Result
	if len(active) > 0 {
		client, err := clientFactory()
		if err != nil {
			return err
		}
		results = client.FetchBatch(context.Background(), active)
	}
	st := data.ComputeStats(results, len(reviewed))
	if jsonOut {
		enc := json.NewEncoder(cmd.OutOrStdout())
		enc.SetIndent("", "  ")
		return enc.Encode(st)
	}
	renderStats(cmd.OutOrStdout(), st)
	return nil
}

func renderStats(out io.Writer, st data.Stats) {
	plain := ui.IsPlain(out)
	fmt.Fprintf(out, "%s\n", ui.Title(plain, "prowl · usage"))
	fmt.Fprintln(out)
	fmt.Fprintf(out, "  tracked total : %d\n", st.Total)
	fmt.Fprintf(out, "  active        : %d\n", st.Active)
	fmt.Fprintf(out, "  reviewed      : %d\n", st.Reviewed)
	if st.Active == 0 {
		return
	}
	fmt.Fprintln(out)
	fmt.Fprintln(out, "  active breakdown:")
	fmt.Fprintf(out, "    %s : %d\n", ui.StatusBadge(plain, "open"), st.Open)
	fmt.Fprintf(out, "    %s : %d\n", ui.StatusBadge(plain, "draft"), st.Draft)
	fmt.Fprintf(out, "    %s : %d\n", ui.StatusBadge(plain, "open/blocked"), st.Blocked)
	fmt.Fprintf(out, "    %s : %d\n", ui.StatusBadge(plain, "merged"), st.Merged)
	fmt.Fprintf(out, "    %s : %d\n", ui.StatusBadge(plain, "closed"), st.Closed)
	if st.Queued > 0 {
		fmt.Fprintf(out, "    queued        : %d\n", st.Queued)
	}
	if st.Errors > 0 {
		fmt.Fprintf(out, "    %s %d\n", ui.Warn(plain), st.Errors)
	}
}
