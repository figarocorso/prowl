package tui

import (
	"fmt"
	"os/exec"
	"runtime"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/spinner"
	"github.com/charmbracelet/bubbles/table"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/figarocorso/prowl/internal/config"
	"github.com/figarocorso/prowl/internal/data"
	"github.com/figarocorso/prowl/internal/store"
)

// autoRefreshTickMsg fires every refreshInterval to trigger a background fetch.
type autoRefreshTickMsg time.Time

var (
	headerStyle  = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("212"))
	statusStyle  = lipgloss.NewStyle().Foreground(lipgloss.Color("245"))
	errStyle     = lipgloss.NewStyle().Foreground(lipgloss.Color("196"))
	okStyle      = lipgloss.NewStyle().Foreground(lipgloss.Color("46"))
	mergedStyle  = lipgloss.NewStyle().Foreground(lipgloss.Color("141"))
	closedStyle  = lipgloss.NewStyle().Foreground(lipgloss.Color("203"))
	keyStyle     = lipgloss.NewStyle().Foreground(lipgloss.Color("212")).Bold(true)
	hintStyle    = lipgloss.NewStyle().Faint(true)
	confirmStyle = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("214"))
)

// statusEmojiLabel returns an emoji-prefixed label for a status string,
// without ANSI escapes so the bubbles table can truncate it correctly.
func statusEmojiLabel(label string) string {
	switch label {
	case "open":
		return "🟢 open"
	case "draft":
		return "📝 draft"
	case "open/blocked":
		return "⛔ blocked"
	case "merged":
		return "🟣 merged"
	case "closed":
		return "🔴 closed"
	case "unknown":
		return "❓ unknown"
	case "error":
		return "⚠ error"
	default:
		return label
	}
}

// Model is the Bubble Tea state for prowl's TUI.
type Model struct {
	cfg             *config.Config
	store           *store.Store
	client          data.PRClient
	table           table.Model
	spinner         spinner.Model
	rows            []data.Result
	loading         bool
	status          string
	err             string
	width           int
	height          int
	confirmArchive  bool
	pendingArchive  []string
	confirmDelete   bool
	pendingDelete   string
	refreshInterval time.Duration
	palette         bool
	paletteInput    string
	overlay         string
}

// SetRefreshInterval enables `prowl watch`-style auto-refresh by scheduling
// a background fetch every d. A non-positive d disables auto-refresh.
func (m *Model) SetRefreshInterval(d time.Duration) {
	if d <= 0 {
		m.refreshInterval = 0
		return
	}
	m.refreshInterval = d
}

// autoRefreshCmd schedules the next auto-refresh tick. Returns nil when
// auto-refresh is disabled so callers can compose it unconditionally.
func (m *Model) autoRefreshCmd() tea.Cmd {
	if m.refreshInterval <= 0 {
		return nil
	}
	return tea.Tick(m.refreshInterval, func(t time.Time) tea.Msg {
		return autoRefreshTickMsg(t)
	})
}

// New builds an unstarted Model.
func New(cfg *config.Config, s *store.Store, client data.PRClient) *Model {
	sp := spinner.New()
	sp.Spinner = spinner.Dot
	sp.Style = lipgloss.NewStyle().Foreground(lipgloss.Color("212"))

	t := table.New(
		table.WithColumns(tableColumns()),
		table.WithFocused(true),
		table.WithHeight(10),
	)
	st := table.DefaultStyles()
	st.Header = st.Header.
		BorderStyle(lipgloss.NormalBorder()).
		BorderForeground(lipgloss.Color("212")).
		BorderTop(false).
		BorderLeft(false).
		BorderRight(false).
		BorderBottom(true).
		Bold(true)
	st.Selected = st.Selected.Foreground(lipgloss.Color("46")).Bold(true)
	t.SetStyles(st)

	return &Model{
		cfg:     cfg,
		store:   s,
		client:  client,
		table:   t,
		spinner: sp,
		loading: true,
		status:  "Loading…",
	}
}

var columnHeaders = []string{"URL", "Assignee", "Status", "Queue", "Pos", "ETA", "Title"}

// columnPad is the extra breathing room added to each non-Title column on top
// of its widest cell.
const columnPad = 2

// minTitleWidth keeps the Title column readable when the terminal is narrow.
const minTitleWidth = 20

// tableColumns returns the initial column set used before the first row fetch
// completes. recomputeColumnWidths replaces them once row content + terminal
// width are known.
func tableColumns() []table.Column {
	cols := make([]table.Column, len(columnHeaders))
	for i, h := range columnHeaders {
		cols[i] = table.Column{Title: h, Width: lipgloss.Width(h) + columnPad}
	}
	return cols
}

// recomputeColumnWidths sizes each non-Title column to fit its widest cell
// (or just its header when the column has no real content), then hands the
// remaining terminal width to the Title column.
func (m *Model) recomputeColumnWidths() {
	widths := make([]int, len(columnHeaders))
	hasContent := make([]bool, len(columnHeaders))
	for i, h := range columnHeaders {
		widths[i] = lipgloss.Width(h)
	}
	for _, r := range m.rows {
		for i, c := range resultCells(r) {
			if w := lipgloss.Width(c); w > widths[i] {
				widths[i] = w
			}
			if c != "" && c != "-" {
				hasContent[i] = true
			}
		}
	}
	for i, h := range columnHeaders {
		if !hasContent[i] {
			widths[i] = lipgloss.Width(h)
		}
		widths[i] += columnPad
	}
	titleIdx := len(columnHeaders) - 1
	if m.width > 0 {
		used := 0
		for i, w := range widths {
			if i == titleIdx {
				continue
			}
			used += w
		}
		// bubbles table reserves a leading space per column; leave a small slack
		// so the last column doesn't overflow the terminal.
		remaining := m.width - used - len(columnHeaders) - 1
		if remaining > widths[titleIdx] {
			widths[titleIdx] = remaining
		}
		if widths[titleIdx] < minTitleWidth {
			widths[titleIdx] = minTitleWidth
		}
	}
	cols := make([]table.Column, len(columnHeaders))
	for i, h := range columnHeaders {
		cols[i] = table.Column{Title: h, Width: widths[i]}
	}
	m.table.SetColumns(cols)
}

func (m *Model) Init() tea.Cmd {
	cmds := []tea.Cmd{m.spinner.Tick, fetchActiveCmd(m.store, m.client)}
	if tick := m.autoRefreshCmd(); tick != nil {
		cmds = append(cmds, tick)
	}
	return tea.Batch(cmds...)
}

func (m *Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		m.table.SetHeight(maxInt(msg.Height-6, 5))
		m.recomputeColumnWidths()
	case tea.KeyMsg:
		if m.palette && !m.confirmArchive && !m.confirmDelete {
			model, cmd := m.handlePaletteKey(msg)
			return model, cmd
		}
		if model, cmd, handled := m.handleKey(msg.String()); handled {
			return model, cmd
		}
	case rowsReadyMsg:
		m.handleRowsReady(msg)
		return m, nil
	case autoRefreshTickMsg:
		return m, m.handleAutoRefresh()
	case spinner.TickMsg:
		var cmd tea.Cmd
		m.spinner, cmd = m.spinner.Update(msg)
		if m.loading {
			return m, cmd
		}
		return m, nil
	}

	var cmd tea.Cmd
	m.table, cmd = m.table.Update(msg)
	return m, cmd
}

func (m *Model) handleKey(key string) (tea.Model, tea.Cmd, bool) {
	if m.confirmArchive {
		model, cmd := m.handleArchiveConfirm(key)
		return model, cmd, true
	}
	if m.confirmDelete {
		model, cmd := m.handleDeleteConfirm(key)
		return model, cmd, true
	}
	return m.handleNormalKey(key)
}

func (m *Model) handleArchiveConfirm(key string) (tea.Model, tea.Cmd) {
	switch key {
	case "y", "Y":
		if _, err := m.store.MoveActiveToReviewed(m.pendingArchive); err != nil {
			m.err = err.Error()
			m.confirmArchive = false
			m.pendingArchive = nil
			return m, nil
		}
		return m, tea.Quit
	case "n", "N", "q", "ctrl+c", "esc", "enter":
		return m, tea.Quit
	}
	return m, nil
}

func (m *Model) handleDeleteConfirm(key string) (tea.Model, tea.Cmd) {
	switch key {
	case "y", "Y":
		url := m.pendingDelete
		m.confirmDelete = false
		m.pendingDelete = ""
		if _, err := m.store.Remove(url); err != nil {
			m.err = err.Error()
			return m, nil
		}
		m.status = "Removed " + url
		kept := m.rows[:0:0]
		for _, r := range m.rows {
			if r.URL != url {
				kept = append(kept, r)
			}
		}
		m.rows = kept
		m.table.SetRows(rowsToTableRows(m.rows))
		m.loading = true
		return m, tea.Batch(m.spinner.Tick, fetchActiveCmd(m.store, m.client))
	case "n", "N", "esc":
		m.confirmDelete = false
		m.pendingDelete = ""
		m.status = "Delete cancelled"
		return m, nil
	}
	return m, nil
}

// handlePaletteKey routes keystrokes typed inside the slash-command palette.
// Enter runs the command, esc cancels, backspace edits. Plain text (single
// keystrokes and clipboard pastes alike, both delivered as KeyRunes) is
// appended verbatim.
func (m *Model) handlePaletteKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.Type {
	case tea.KeyEsc, tea.KeyCtrlC:
		m.palette = false
		m.paletteInput = ""
		m.status = "Palette closed"
		return m, nil
	case tea.KeyEnter:
		input := strings.TrimSpace(m.paletteInput)
		m.palette = false
		m.paletteInput = ""
		return m.runPaletteCommand(input)
	case tea.KeyBackspace:
		if r := []rune(m.paletteInput); len(r) > 0 {
			m.paletteInput = string(r[:len(r)-1])
		}
		return m, nil
	case tea.KeySpace:
		m.paletteInput += " "
		return m, nil
	case tea.KeyRunes:
		m.paletteInput += string(msg.Runes)
		return m, nil
	}
	return m, nil
}

// runPaletteCommand parses and dispatches a palette command. The leading
// slash, if any, is tolerated so users can type either `add ...` or `/add ...`.
func (m *Model) runPaletteCommand(input string) (tea.Model, tea.Cmd) {
	input = strings.TrimPrefix(input, "/")
	if input == "" {
		m.status = "Empty command"
		return m, nil
	}
	parts := strings.Fields(input)
	cmd := strings.ToLower(parts[0])
	args := parts[1:]
	switch cmd {
	case "add":
		return m.runPaletteAdd(args)
	case "usage", "stats":
		return m.runPaletteUsage()
	case "archive", "clean":
		return m.runPaletteArchive()
	default:
		m.err = "unknown command: " + cmd
		m.status = "Try: add <url> · usage · archive"
		return m, nil
	}
}

func (m *Model) runPaletteAdd(args []string) (tea.Model, tea.Cmd) {
	if len(args) == 0 {
		m.err = "usage: add <pr_url>"
		return m, nil
	}
	canonical, err := data.CanonicalURL(args[0])
	if err != nil {
		m.err = err.Error()
		return m, nil
	}
	added, err := m.store.Add(canonical)
	if err != nil {
		m.err = err.Error()
		return m, nil
	}
	if !added {
		m.status = "Already tracked: " + canonical
		return m, nil
	}
	m.status = "Added " + canonical + " — refreshing…"
	m.err = ""
	m.loading = true
	return m, tea.Batch(m.spinner.Tick, fetchActiveCmd(m.store, m.client))
}

func (m *Model) runPaletteUsage() (tea.Model, tea.Cmd) {
	reviewed, err := m.store.Reviewed()
	if err != nil {
		m.err = err.Error()
		return m, nil
	}
	stats := data.ComputeStats(m.rows, len(reviewed))
	m.overlay = formatUsage(stats)
	m.status = "Usage overlay - press esc to close"
	return m, nil
}

func (m *Model) runPaletteArchive() (tea.Model, tea.Cmd) {
	terminal := m.terminalURLs()
	if len(terminal) == 0 {
		m.status = "Nothing to archive"
		return m, nil
	}
	moved, err := m.store.MoveActiveToReviewed(terminal)
	if err != nil {
		m.err = err.Error()
		return m, nil
	}
	m.status = fmt.Sprintf("Archived %d PR(s) — refreshing…", moved)
	m.err = ""
	m.loading = true
	return m, tea.Batch(m.spinner.Tick, fetchActiveCmd(m.store, m.client))
}

func formatUsage(s data.Stats) string {
	var b strings.Builder
	b.WriteString(okStyle.Render("📊 prowl usage") + "\n\n")
	fmt.Fprintf(&b, "  tracked total : %d\n", s.Total)
	fmt.Fprintf(&b, "  active        : %d\n", s.Active)
	fmt.Fprintf(&b, "  reviewed      : %d\n\n", s.Reviewed)
	fmt.Fprintf(&b, "  🟢 open        : %d\n", s.Open)
	fmt.Fprintf(&b, "  📝 draft       : %d\n", s.Draft)
	fmt.Fprintf(&b, "  ⛔ blocked     : %d\n", s.Blocked)
	fmt.Fprintf(&b, "  🟣 merged      : %d\n", s.Merged)
	fmt.Fprintf(&b, "  🔴 closed      : %d\n", s.Closed)
	if s.Queued > 0 {
		fmt.Fprintf(&b, "  🚦 queued      : %d\n", s.Queued)
	}
	if s.Errors > 0 {
		fmt.Fprintf(&b, "  ⚠ errors      : %d\n", s.Errors)
	}
	return b.String()
}

// handleAutoRefresh re-arms the auto-refresh tick and, when idle, kicks
// off a background fetch. Returns the next tea.Cmd to run.
func (m *Model) handleAutoRefresh() tea.Cmd {
	next := m.autoRefreshCmd()
	if next == nil {
		return nil
	}
	if m.loading || m.confirmArchive || m.confirmDelete {
		return next
	}
	m.loading = true
	m.status = "Auto-refreshing…"
	return tea.Batch(m.spinner.Tick, fetchActiveCmd(m.store, m.client), next)
}

func (m *Model) handleNormalKey(key string) (tea.Model, tea.Cmd, bool) {
	switch key {
	case "/", ":":
		m.palette = true
		m.paletteInput = ""
		m.overlay = ""
		m.err = ""
		m.status = "Command palette — type add <url>, usage, archive · esc cancels"
		return m, nil, true
	case "esc":
		if m.overlay != "" {
			m.overlay = ""
			m.status = "Overlay closed"
			return m, nil, true
		}
		terminal := m.terminalURLs()
		if len(terminal) == 0 {
			return m, tea.Quit, true
		}
		m.confirmArchive = true
		m.pendingArchive = terminal
		return m, nil, true
	case "q", "ctrl+c":
		terminal := m.terminalURLs()
		if len(terminal) == 0 {
			return m, tea.Quit, true
		}
		m.confirmArchive = true
		m.pendingArchive = terminal
		return m, nil, true
	case "r", "ctrl+r":
		if !m.loading {
			m.loading = true
			m.status = "Refreshing…"
			return m, tea.Batch(m.spinner.Tick, fetchActiveCmd(m.store, m.client)), true
		}
	case "enter":
		if url := m.selectedURL(); url != "" {
			_ = openInBrowser(url)
		}
	case "c":
		if url := m.selectedURL(); url != "" {
			if err := copyToClipboard(url); err != nil {
				m.err = err.Error()
			} else {
				m.status = "Copied " + url
			}
		}
	case "d", "backspace", "delete":
		if url := m.selectedURL(); url != "" {
			m.confirmDelete = true
			m.pendingDelete = url
			return m, nil, true
		}
	}
	return m, nil, false
}

func (m *Model) handleRowsReady(msg rowsReadyMsg) {
	m.loading = false
	if msg.err != nil {
		m.err = msg.err.Error()
		m.rows = nil
		m.table.SetRows(nil)
		return
	}
	m.err = ""
	m.rows = msg.results
	m.table.SetRows(rowsToTableRows(msg.results))
	m.recomputeColumnWidths()
	m.status = summary(msg.results)
}

func (m *Model) View() string {
	var b strings.Builder
	b.WriteString(headerStyle.Render("🦉 prowl"))
	b.WriteString("  ")
	if m.loading {
		b.WriteString(m.spinner.View())
		b.WriteString(" ")
	}
	b.WriteString(statusStyle.Render(m.status))
	b.WriteString("\n\n")

	if m.err != "" {
		b.WriteString(errStyle.Render("✗ " + m.err))
		b.WriteString("\n\n")
	}

	if len(m.rows) == 0 && !m.loading {
		b.WriteString(okStyle.Render("📭 no active PRs — `prowl add <url>` or press `/` then `add <url>`\n"))
	} else {
		b.WriteString(m.table.View())
		b.WriteString("\n")
	}

	if m.overlay != "" {
		b.WriteString("\n")
		b.WriteString(m.overlay)
	}

	switch {
	case m.confirmArchive:
		prompt := fmt.Sprintf("\n📦 Archive %d closed/merged PR(s)? [y/N]", len(m.pendingArchive))
		b.WriteString(confirmStyle.Render(prompt))
	case m.confirmDelete:
		prompt := fmt.Sprintf("\n🗑  Delete %s? [y/N]", m.pendingDelete)
		b.WriteString(confirmStyle.Render(prompt))
	case m.palette:
		b.WriteString("\n" + confirmStyle.Render("/"+m.paletteInput+"▌"))
		b.WriteString(hintStyle.Render("   (commands: add <url>, usage, archive · esc cancels)"))
	default:
		hints := []string{
			keyStyle.Render("↑↓") + hintStyle.Render(" nav"),
			keyStyle.Render("⏎") + hintStyle.Render(" open"),
			keyStyle.Render("c") + hintStyle.Render(" copy"),
			keyStyle.Render("d") + hintStyle.Render(" delete"),
			keyStyle.Render("r") + hintStyle.Render(" refresh"),
			keyStyle.Render("/") + hintStyle.Render(" cmd"),
			keyStyle.Render("q") + hintStyle.Render(" quit"),
		}
		b.WriteString("\n" + strings.Join(hints, hintStyle.Render(" · ")))
	}
	return b.String()
}

func (m *Model) terminalURLs() []string {
	var out []string
	for _, r := range m.rows {
		if r.Err != nil {
			continue
		}
		if data.IsTerminal(r.PR) {
			out = append(out, r.URL)
		}
	}
	return out
}

func (m *Model) selectedURL() string {
	cursor := m.table.Cursor()
	if cursor < 0 || cursor >= len(m.rows) {
		return ""
	}
	return m.rows[cursor].URL
}

// resultCells returns the raw column values for a single result, in the order
// declared by columnHeaders.
func resultCells(r data.Result) []string {
	if r.Err != nil {
		return []string{data.ShortURL(r.URL), "-", statusEmojiLabel("error"), "-", "-", "-", "-"}
	}
	pr := r.PR
	return []string{
		data.ShortURL(pr.URL),
		data.AssigneesLabel(pr),
		statusEmojiLabel(data.StatusLabel(pr)),
		data.QueueLabelShort(pr),
		data.QueuePositionLabel(pr),
		data.ETALabel(pr),
		pr.Title,
	}
}

func rowsToTableRows(results []data.Result) []table.Row {
	out := make([]table.Row, 0, len(results))
	for _, r := range results {
		cells := resultCells(r)
		row := make(table.Row, len(cells))
		copy(row, cells)
		out = append(out, row)
	}
	return out
}

func summary(results []data.Result) string {
	open, merged, closed, errs := 0, 0, 0, 0
	for _, r := range results {
		if r.Err != nil {
			errs++
			continue
		}
		switch strings.ToUpper(r.PR.State) {
		case "OPEN":
			open++
		case "MERGED":
			merged++
		case "CLOSED":
			closed++
		}
	}
	parts := []string{
		"📊",
		okStyle.Render(fmt.Sprintf("🟢 %d open", open)),
		statusStyle.Render("·"),
		mergedStyle.Render(fmt.Sprintf("🟣 %d merged", merged)),
		statusStyle.Render("·"),
		closedStyle.Render(fmt.Sprintf("🔴 %d closed", closed)),
	}
	s := strings.Join(parts, " ")
	if errs > 0 {
		s += " " + statusStyle.Render("·") + " " + errStyle.Render(fmt.Sprintf("⚠ %d errors", errs))
	}
	return s
}

func openInBrowser(url string) error {
	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "darwin":
		cmd = exec.Command("open", url)
	case "windows":
		cmd = exec.Command("rundll32", "url.dll,FileProtocolHandler", url)
	default:
		cmd = exec.Command("xdg-open", url)
	}
	return cmd.Start()
}

func copyToClipboard(s string) error {
	candidates := [][]string{
		{"pbcopy"},
		{"wl-copy"},
		{"xclip", "-selection", "clipboard"},
		{"xsel", "--clipboard", "--input"},
	}
	for _, c := range candidates {
		if _, err := exec.LookPath(c[0]); err != nil {
			continue
		}
		cmd := exec.Command(c[0], c[1:]...)
		stdin, err := cmd.StdinPipe()
		if err != nil {
			return err
		}
		if err := cmd.Start(); err != nil {
			return err
		}
		if _, err := stdin.Write([]byte(s)); err != nil {
			return err
		}
		if err := stdin.Close(); err != nil {
			return err
		}
		return cmd.Wait()
	}
	return fmt.Errorf("no clipboard tool found")
}

func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}
