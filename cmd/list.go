package cmd

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/figarocorso/prowl/internal/data"
	"github.com/figarocorso/prowl/internal/store"
	"github.com/figarocorso/prowl/internal/ui"
	"github.com/spf13/cobra"
	"golang.org/x/term"
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

// titleFallbackWidth is used to cap titles when the output isn't a TTY (e.g.
// piped output) so dumps stay readable without knowing the terminal width.
const titleFallbackWidth = 80

// minTitleWidth keeps the Title column readable when the terminal is narrow.
const minTitleWidth = 20

func renderTable(out io.Writer, results []data.Result) error {
	plain := ui.IsPlain(out)
	headers := []string{"URL", "Assignee", "Status", "Queue", "Pos", "ETA", "Title"}
	rows := buildTableRows(plain, headers, results)
	widths := smartColumnWidths(rows, headers, terminalWidth(out))
	for _, row := range rows {
		printTableRow(out, row, widths)
	}
	return nil
}

// terminalWidth returns the column count of w when w is a TTY, 0 otherwise.
func terminalWidth(w io.Writer) int {
	f, ok := w.(*os.File)
	if !ok {
		return 0
	}
	if !term.IsTerminal(int(f.Fd())) {
		return 0
	}
	cols, _, err := term.GetSize(int(f.Fd()))
	if err != nil {
		return 0
	}
	return cols
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
		return [7]string{data.ShortURL(r.URL), "-", "error", "-", "-", "-", "-"}
	}
	pr := r.PR
	return [7]string{
		data.ShortURL(pr.URL),
		data.AssigneesLabel(pr),
		data.StatusLabel(pr),
		data.QueueLabelShort(pr),
		data.QueuePositionLabel(pr),
		data.ETALabel(pr),
		pr.Title,
	}
}

func renderCellValue(plain bool, col int, v string) string {
	switch col {
	case 0:
		return ui.Dim(plain, v)
	case 2:
		return ui.StatusBadge(plain, v)
	case 6:
		return v
	default:
		return v
	}
}

// truncate shortens s to n runes max, appending an ellipsis when truncated.
func truncate(s string, n int) string {
	if n <= 0 {
		return ""
	}
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	if n == 1 {
		return "…"
	}
	return string(r[:n-1]) + "…"
}

// smartColumnWidths sizes each column to fit the widest cell content (or just
// the header when the column has no real value), then either lets the last
// (Title) column flow to the end of termWidth or — when termWidth is 0 (e.g.
// the output is piped) — caps it at titleFallbackWidth.
func smartColumnWidths(rows [][]tableCell, headers []string, termWidth int) []int {
	if len(rows) == 0 {
		return nil
	}
	cols := len(rows[0])
	widths := make([]int, cols)
	hasContent := make([]bool, cols)
	for i, h := range headers {
		widths[i] = lipgloss.Width(h)
	}
	// rows[0] is the header — skip it when looking for content.
	for _, row := range rows[1:] {
		for i, c := range row {
			if w := lipgloss.Width(c.raw); w > widths[i] {
				widths[i] = w
			}
			if c.raw != "" && c.raw != "-" {
				hasContent[i] = true
			}
		}
	}
	for i, h := range headers {
		if !hasContent[i] {
			widths[i] = lipgloss.Width(h)
		}
	}
	titleIdx := cols - 1
	// Title gets the leftover terminal width when we can detect a TTY; the
	// fallback cap keeps piped output sane.
	maxTitle := 0
	for _, row := range rows[1:] {
		if w := lipgloss.Width(row[titleIdx].raw); w > maxTitle {
			maxTitle = w
		}
	}
	if termWidth > 0 {
		used := 0
		for i, w := range widths {
			if i == titleIdx {
				continue
			}
			used += w + 2 // gap between columns
		}
		remaining := termWidth - used
		remaining = max(remaining, minTitleWidth)
		widths[titleIdx] = min(remaining, maxTitle)
	} else {
		widths[titleIdx] = min(maxTitle, titleFallbackWidth)
	}
	widths[titleIdx] = max(widths[titleIdx], lipgloss.Width(headers[titleIdx]))
	return widths
}

func printTableRow(out io.Writer, row []tableCell, widths []int) {
	for i, c := range row {
		if i == len(row)-1 {
			fmt.Fprint(out, truncateCell(c.rendered, c.raw, widths[i]))
			continue
		}
		pad := max(widths[i]-lipgloss.Width(c.raw)+2, 1)
		fmt.Fprint(out, c.rendered, strings.Repeat(" ", pad))
	}
	fmt.Fprintln(out)
}

// truncateCell shortens rendered to width runes when raw exceeds it. ANSI
// styling on rendered would break under naive slicing, so we re-render from
// the truncated raw form.
func truncateCell(rendered, raw string, width int) string {
	if lipgloss.Width(raw) <= width {
		return rendered
	}
	return truncate(raw, width)
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
