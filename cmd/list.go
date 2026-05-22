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

type tableCell struct{ raw, rendered string }

func renderTable(out io.Writer, results []data.Result) error {
	plain := ui.IsPlain(out)
	headers := []string{"PR", "Assignee", "Status", "Queue", "Pos", "ETA", "URL"}
	rows := buildTableRows(plain, headers, results)
	widths := columnWidths(rows)
	for _, row := range rows {
		printTableRow(out, row, widths)
	}
	return nil
}

func buildTableRows(plain bool, headers []string, results []data.Result) [][]tableCell {
	rows := make([][]tableCell, 0, len(results)+1)
	header := make([]tableCell, len(headers))
	for i, h := range headers {
		header[i] = tableCell{raw: h, rendered: ui.Header(plain, h)}
	}
	rows = append(rows, header)
	for _, r := range results {
		rows = append(rows, resultToRow(plain, r))
	}
	return rows
}

func resultToRow(plain bool, r data.Result) []tableCell {
	raw := rawRowValues(r)
	row := make([]tableCell, len(raw))
	for i, v := range raw {
		row[i] = tableCell{raw: v, rendered: renderCellValue(plain, i, v)}
	}
	return row
}

func rawRowValues(r data.Result) [7]string {
	if r.Err != nil {
		return [7]string{"?", "-", "error", "-", "-", "-", r.URL}
	}
	pr := r.PR
	num := "?"
	if pr.Number > 0 {
		num = fmt.Sprintf("#%d", pr.Number)
	}
	return [7]string{
		num,
		data.AssigneesLabel(pr),
		data.StatusLabel(pr),
		data.QueueLabel(pr),
		data.QueuePositionLabel(pr),
		data.ETALabel(pr),
		pr.URL,
	}
}

func renderCellValue(plain bool, col int, v string) string {
	switch col {
	case 2:
		return ui.StatusBadge(plain, v)
	case 6:
		return ui.Dim(plain, v)
	default:
		return v
	}
}

func columnWidths(rows [][]tableCell) []int {
	if len(rows) == 0 {
		return nil
	}
	widths := make([]int, len(rows[0]))
	for _, row := range rows {
		for i, c := range row {
			if w := lipgloss.Width(c.raw); w > widths[i] {
				widths[i] = w
			}
		}
	}
	return widths
}

func printTableRow(out io.Writer, row []tableCell, widths []int) {
	for i, c := range row {
		if i == len(row)-1 {
			fmt.Fprint(out, c.rendered)
			continue
		}
		pad := max(widths[i]-lipgloss.Width(c.raw)+2, 1)
		fmt.Fprint(out, c.rendered, strings.Repeat(" ", pad))
	}
	fmt.Fprintln(out)
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
