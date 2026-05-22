package cmd

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/figarocorso/prowl/internal/data"
	"github.com/figarocorso/prowl/internal/store"
	"github.com/figarocorso/prowl/internal/ui"
	"github.com/spf13/cobra"
)

var (
	listSource   string
	listOpenOnly bool
)

var listCmd = &cobra.Command{
	Use:   "list",
	Short: "Print tracked PRs as a table",
	RunE:  runList,
}

func init() {
	listCmd.Flags().StringVar(&listSource, "source", "active", "which list to read: active | reviewed | all")
	listCmd.Flags().BoolVar(&listOpenOnly, "open", false, "only print PRs whose current state is OPEN")
	listCmd.Flags().BoolVar(&jsonOut, "json", false, "emit a JSON array instead of a table")
	rootCmd.AddCommand(listCmd)
}

func runList(cmd *cobra.Command, _ []string) error {
	_, s, err := loadConfigAndStore()
	if err != nil {
		return err
	}
	urls, err := selectURLs(s, listSource)
	if err != nil {
		return err
	}
	if len(urls) == 0 {
		plain := ui.IsPlain(cmd.OutOrStderr())
		if plain {
			fmt.Fprintln(cmd.OutOrStderr(), "no PRs to list")
		} else {
			fmt.Fprintln(cmd.OutOrStderr(), "📭 no PRs to list")
		}
		return nil
	}
	client, err := clientFactory()
	if err != nil {
		return err
	}
	results := client.FetchBatch(context.Background(), urls)
	if listOpenOnly {
		results = filterOpen(results)
	}
	if jsonOut {
		return emitJSON(cmd.OutOrStdout(), results)
	}
	return renderTable(cmd.OutOrStdout(), results)
}

func selectURLs(s *store.Store, source string) ([]string, error) {
	switch strings.ToLower(source) {
	case "active", "":
		return s.Active()
	case "reviewed":
		return s.Reviewed()
	case "all":
		a, err := s.Active()
		if err != nil {
			return nil, err
		}
		r, err := s.Reviewed()
		if err != nil {
			return nil, err
		}
		return append(a, r...), nil
	default:
		return nil, fmt.Errorf("invalid --source %q (active|reviewed|all)", source)
	}
}

func filterOpen(in []data.Result) []data.Result {
	var out []data.Result
	for _, r := range in {
		if r.Err != nil {
			out = append(out, r)
			continue
		}
		if strings.EqualFold(r.PR.State, "OPEN") {
			out = append(out, r)
		}
	}
	return out
}

func renderTable(out io.Writer, results []data.Result) error {
	plain := ui.IsPlain(out)
	headers := []string{"PR", "Assignee", "Status", "Queue", "Pos", "ETA", "URL"}

	type cell struct{ raw, rendered string }
	rows := make([][]cell, 0, len(results)+1)

	headerRow := make([]cell, len(headers))
	for i, h := range headers {
		headerRow[i] = cell{raw: h, rendered: ui.Header(plain, h)}
	}
	rows = append(rows, headerRow)

	for _, r := range results {
		var raw [7]string
		if r.Err != nil {
			raw = [7]string{"?", "-", "error", "-", "-", "-", r.URL}
		} else {
			pr := r.PR
			num := "?"
			if pr.Number > 0 {
				num = fmt.Sprintf("#%d", pr.Number)
			}
			raw = [7]string{
				num,
				data.AssigneesLabel(pr),
				data.StatusLabel(pr),
				data.QueueLabel(pr),
				data.QueuePositionLabel(pr),
				data.ETALabel(pr),
				pr.URL,
			}
		}
		row := make([]cell, 7)
		for i, v := range raw {
			rendered := v
			switch i {
			case 2:
				rendered = ui.StatusBadge(plain, v)
			case 6:
				rendered = ui.Dim(plain, v)
			}
			row[i] = cell{raw: v, rendered: rendered}
		}
		rows = append(rows, row)
	}

	widths := make([]int, len(headers))
	for _, row := range rows {
		for i, c := range row {
			if w := lipgloss.Width(c.raw); w > widths[i] {
				widths[i] = w
			}
		}
	}

	for _, row := range rows {
		for i, c := range row {
			if i == len(row)-1 {
				fmt.Fprint(out, c.rendered)
				continue
			}
			pad := widths[i] - lipgloss.Width(c.raw) + 2
			if pad < 1 {
				pad = 1
			}
			fmt.Fprint(out, c.rendered, strings.Repeat(" ", pad))
		}
		fmt.Fprintln(out)
	}
	return nil
}

type jsonRow struct {
	URL        string   `json:"url"`
	Number     int      `json:"number,omitempty"`
	Title      string   `json:"title,omitempty"`
	State      string   `json:"state,omitempty"`
	Status     string   `json:"status,omitempty"`
	IsDraft    bool     `json:"is_draft,omitempty"`
	Assignees  []string `json:"assignees,omitempty"`
	QueueState string   `json:"queue_state,omitempty"`
	QueuePos   int      `json:"queue_position,omitempty"`
	QueueETA   string   `json:"queue_eta,omitempty"`
	Error      string   `json:"error,omitempty"`
}

func emitJSON(out io.Writer, results []data.Result) error {
	rows := make([]jsonRow, 0, len(results))
	for _, r := range results {
		row := jsonRow{URL: r.URL}
		if r.Err != nil {
			row.Error = r.Err.Error()
			rows = append(rows, row)
			continue
		}
		pr := r.PR
		row.Number = pr.Number
		row.Title = pr.Title
		row.State = pr.State
		row.Status = data.StatusLabel(pr)
		row.IsDraft = pr.IsDraft
		row.Assignees = pr.Assignees
		if pr.Queue != nil {
			row.QueueState = pr.Queue.State
			row.QueuePos = pr.Queue.Position
			if pr.Queue.ETA > 0 {
				row.QueueETA = pr.Queue.ETA.String()
			}
		}
		rows = append(rows, row)
	}
	enc := json.NewEncoder(out)
	enc.SetIndent("", "  ")
	return enc.Encode(rows)
}
