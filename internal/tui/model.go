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
	statusStyle  = lipgloss.NewStyle().Foreground(lipgloss.Color("245"))
	errStyle     = lipgloss.NewStyle().Foreground(lipgloss.Color("196"))
	okStyle      = lipgloss.NewStyle().Foreground(lipgloss.Color("46"))
	mergedStyle  = lipgloss.NewStyle().Foreground(lipgloss.Color("141"))
	closedStyle  = lipgloss.NewStyle().Foreground(lipgloss.Color("203"))
	keyStyle     = lipgloss.NewStyle().Foreground(lipgloss.Color("212")).Bold(true)
	hintStyle    = lipgloss.NewStyle().Faint(true)
	confirmStyle = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("214"))
	titleStyle   = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("231")).Background(lipgloss.Color("39")).Padding(0, 1)
	dividerStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("240"))
	activeTab    = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("231")).Background(lipgloss.Color("212")).Padding(0, 2)
	inactiveTab  = lipgloss.NewStyle().Foreground(lipgloss.Color("245")).Padding(0, 2)
)

// tabID identifies a top-level view tab.
type tabID int

const (
	tabActive tabID = iota
	tabDone
)

var tabTitles = []string{"Active", "Done"}

// paletteCommands lists the canonical command names offered for tab-completion
// in the slash-command palette, scoped to the active tab (aliases like
// "stats"/"clean" are intentionally omitted so completion suggests one name per
// action).
func (m *Model) paletteCommands() []string {
	if m.tab == tabDone {
		return []string{"search"}
	}
	return []string{"add", "archive", "usage"}
}

// paletteSuggestion returns the completion suffix for the first command whose
// name starts with the typed input. It returns "" once a space has been typed
// (i.e. the user has moved on to arguments) or when nothing matches.
func paletteSuggestion(cmds []string, input string) string {
	input = strings.TrimPrefix(input, "/")
	if input == "" || strings.ContainsRune(input, ' ') {
		return ""
	}
	for _, cmd := range cmds {
		if strings.HasPrefix(cmd, input) && cmd != input {
			return cmd[len(input):]
		}
	}
	return ""
}

// statusEmojiLabel returns an emoji-prefixed label for a status string,
// without ANSI escapes so the bubbles table can truncate it correctly.
func statusEmojiLabel(label string) string {
	switch label {
	case "open":
		return "🟢 open"
	case "queued":
		return "🚦 queued"
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

// reviewedState holds the paginated, newest-first view of closed/merged PRs.
// Rows are fetched lazily a page at a time as the cursor reaches the bottom.
type reviewedState struct {
	table       table.Model
	rows        []data.Result // every fetched result, in pagination order
	shown       []data.Result // results currently backing the table (filtered when searching)
	urls        []string      // all reviewed URLs, newest-first
	loaded      int           // number of urls already fetched into rows
	loading     bool
	initialized bool
	query       string // active /search keyword; "" when not searching
	err         string
}

// Model is the Bubble Tea state for prowl's TUI.
type Model struct {
	cfg             *config.Config
	store           *store.Store
	client          data.PRClient
	tab             tabID
	table           table.Model
	spinner         spinner.Model
	rows            []data.Result
	reviewed        reviewedState
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

	return &Model{
		cfg:      cfg,
		store:    s,
		client:   client,
		table:    newPRTable(),
		reviewed: reviewedState{table: newPRTable()},
		spinner:  sp,
		loading:  true,
		status:   "Loading…",
	}
}

// newPRTable builds a focused PR table with prowl's shared styling.
func newPRTable() table.Model {
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
	return t
}

var columnHeaders = []string{"URL", "Assignee", "Status", "Title"}

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

// recomputeColumnWidths re-sizes both tabs' columns against their own rows.
func (m *Model) recomputeColumnWidths() {
	m.table.SetColumns(m.columnsFor(m.rows))
	m.reviewed.table.SetColumns(m.columnsFor(m.reviewed.rows))
}

// columnsFor sizes each non-Title column to fit its widest cell (or just its
// header when the column has no real content), then hands the remaining
// terminal width to the Title column.
func (m *Model) columnsFor(rows []data.Result) []table.Column {
	widths, hasContent := tuiColWidthsFromRows(rows)
	finalizeTuiColWidths(widths, hasContent)
	titleIdx := len(columnHeaders) - 1
	widths[titleIdx] = expandTitleWidth(widths, titleIdx, m.width)
	cols := make([]table.Column, len(columnHeaders))
	for i, h := range columnHeaders {
		cols[i] = table.Column{Title: h, Width: widths[i]}
	}
	return cols
}

// visibleRows is the height of the table block, i.e. the terminal height minus
// the 6 chrome lines around it: the header, a blank separator, the tab bar,
// another blank line, a blank line above the hints, and the hints themselves.
func (m *Model) visibleRows() int {
	return maxInt(m.height-6, 5)
}

// pageSize is the reviewed-tab page: the lesser of 20 and the data rows that
// fit. The table viewport shows visibleRows minus its 2-line header; reserve
// one more for the "Load more" sentinel so a full page fits without scrolling.
func (m *Model) pageSize() int {
	v := max(m.visibleRows()-3, 1)
	if v > 20 {
		return 20
	}
	return v
}

// tuiColWidthsFromRows seeds widths from columnHeaders and grows each to fit
// the widest rendered cell in rows.
func tuiColWidthsFromRows(rows []data.Result) ([]int, []bool) {
	widths := make([]int, len(columnHeaders))
	hasContent := make([]bool, len(columnHeaders))
	for i, h := range columnHeaders {
		widths[i] = lipgloss.Width(h)
	}
	for _, r := range rows {
		for i, c := range resultCells(r) {
			if w := lipgloss.Width(c); w > widths[i] {
				widths[i] = w
			}
			if c != "" && c != "-" {
				hasContent[i] = true
			}
		}
	}
	return widths, hasContent
}

// finalizeTuiColWidths collapses empty columns to header width and applies
// the per-column breathing pad.
func finalizeTuiColWidths(widths []int, hasContent []bool) {
	for i, h := range columnHeaders {
		if !hasContent[i] {
			widths[i] = lipgloss.Width(h)
		}
		widths[i] += columnPad
	}
}

// expandTitleWidth grows the Title column to soak up any remaining terminal
// width while staying above minTitleWidth.
func expandTitleWidth(widths []int, titleIdx, termWidth int) int {
	if termWidth <= 0 {
		return widths[titleIdx]
	}
	used := 0
	for i, w := range widths {
		if i == titleIdx {
			continue
		}
		used += w
	}
	// bubbles table reserves a leading space per column; leave a small slack
	// so the last column doesn't overflow the terminal.
	remaining := termWidth - used - len(columnHeaders) - 1
	if remaining > widths[titleIdx] {
		widths[titleIdx] = remaining
	}
	return max(widths[titleIdx], minTitleWidth)
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
		h := m.visibleRows()
		m.table.SetHeight(h)
		m.reviewed.table.SetHeight(h)
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
	case reviewedInitMsg:
		m.handleReviewedInit(msg)
		return m, nil
	case reviewedPageMsg:
		m.handleReviewedPage(msg)
		return m, nil
	case autoRefreshTickMsg:
		return m, m.handleAutoRefresh()
	case spinner.TickMsg:
		var cmd tea.Cmd
		m.spinner, cmd = m.spinner.Update(msg)
		if m.loading || m.reviewed.loading {
			return m, cmd
		}
		return m, nil
	}

	// Forward unhandled messages (mostly cursor navigation) to the current
	// tab's table.
	if m.tab == tabDone {
		var cmd tea.Cmd
		m.reviewed.table, cmd = m.reviewed.table.Update(msg)
		return m, cmd
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
	case "y", "Y", "enter":
		if _, err := m.store.MoveActiveToReviewed(m.pendingArchive); err != nil {
			m.err = err.Error()
			m.confirmArchive = false
			m.pendingArchive = nil
			return m, nil
		}
		return m, tea.Quit
	case "n", "N", "q", "ctrl+c", "esc":
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
	case tea.KeyTab:
		m.paletteInput += paletteSuggestion(m.paletteCommands(), m.paletteInput)
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
	case "search":
		return m.runPaletteSearch(args)
	default:
		m.err = "unknown command: " + cmd
		m.status = "Try: add <url> · usage · archive · search <keyword>"
		return m, nil
	}
}

// runPaletteSearch filters the Done tab to the already-loaded PRs whose URL or
// title contains the keyword (case-insensitive). It does not fetch more pages,
// so it is instant; load more rows first to widen the search scope.
func (m *Model) runPaletteSearch(args []string) (tea.Model, tea.Cmd) {
	if m.tab != tabDone {
		m.err = "search is only available on the Done tab"
		return m, nil
	}
	if len(args) == 0 {
		m.err = "usage: search <keyword>"
		return m, nil
	}
	m.err = ""
	m.reviewed.query = strings.Join(args, " ")
	m.reviewed.table.SetCursor(0)
	m.refreshReviewedTable()
	return m, nil
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
	case "left", "h":
		return m.switchTab(tabActive)
	case "right", "l":
		return m.switchTab(tabDone)
	case "/", ":":
		return m.openPalette(), nil, true
	case "esc":
		return m.handleEsc()
	case "q", "ctrl+c":
		return m.quitOrPromptArchive()
	case "r", "ctrl+r":
		if m.tab != tabActive {
			return m, nil, true
		}
		return m.refresh()
	case "enter":
		return m.handleEnter()
	case "down", "j":
		if m.tab == tabDone && m.loadMoreSelected() {
			return m, m.loadMoreReviewed(), true
		}
		return m, nil, false
	case "c":
		m.copySelectedURL()
	case "d", "backspace", "delete":
		return m.handleDeleteKey()
	}
	return m, nil, false
}

// handleEnter loads the next reviewed page when the cursor is on the "Load
// more" sentinel, otherwise opens the selected PR in the browser.
func (m *Model) handleEnter() (tea.Model, tea.Cmd, bool) {
	if m.tab == tabDone && m.loadMoreSelected() {
		return m, m.loadMoreReviewed(), true
	}
	if url := m.selectedURL(); url != "" {
		_ = openInBrowser(url)
	}
	return m, nil, false
}

// handleDeleteKey prompts to delete the selected PR. Delete is only available
// on the Active tab; on other tabs the key is swallowed.
func (m *Model) handleDeleteKey() (tea.Model, tea.Cmd, bool) {
	if m.tab != tabActive {
		return m, nil, true
	}
	if url := m.selectedURL(); url != "" {
		m.confirmDelete = true
		m.pendingDelete = url
		return m, nil, true
	}
	return m, nil, false
}

// switchTab activates t, kicking off the reviewed tab's first fetch the first
// time it is opened.
func (m *Model) switchTab(t tabID) (tea.Model, tea.Cmd, bool) {
	if t == m.tab {
		return m, nil, true
	}
	m.tab = t
	if t == tabDone && !m.reviewed.initialized {
		m.reviewed.initialized = true
		m.reviewed.loading = true
		return m, tea.Batch(m.spinner.Tick, fetchReviewedInitCmd(m.store, m.client, m.pageSize())), true
	}
	return m, nil, true
}

// hasMoreReviewed reports whether unfetched reviewed URLs remain.
func (m *Model) hasMoreReviewed() bool {
	return m.reviewed.loaded < len(m.reviewed.urls)
}

// loadMoreSelected reports whether the cursor sits on the "Load more" sentinel
// row (the synthetic row appended after the loaded results). Never true while
// searching, since the sentinel is hidden then.
func (m *Model) loadMoreSelected() bool {
	return m.reviewed.query == "" && m.hasMoreReviewed() && m.reviewed.table.Cursor() == len(m.reviewed.shown)
}

// loadMoreReviewed fetches the next reviewed page. No-op while a fetch is in
// flight or once every URL has been loaded.
func (m *Model) loadMoreReviewed() tea.Cmd {
	r := &m.reviewed
	if r.loading || !m.hasMoreReviewed() {
		return nil
	}
	end := min(r.loaded+m.pageSize(), len(r.urls))
	page := r.urls[r.loaded:end]
	r.loading = true
	m.refreshReviewedTable() // flip sentinel to its loading label
	return tea.Batch(m.spinner.Tick, fetchReviewedPageCmd(m.client, page))
}

// refreshReviewedTable rebuilds the Done table from the fetched results: the
// filtered subset when a search is active, otherwise the full paginated list
// with a trailing "Load more" sentinel. It also caches the visible results in
// reviewed.shown so cursor lookups map to the right URL.
func (m *Model) refreshReviewedTable() {
	r := &m.reviewed
	if r.query == "" {
		r.shown = r.rows
	} else {
		q := strings.ToLower(r.query)
		shown := make([]data.Result, 0, len(r.rows))
		for _, res := range r.rows {
			if matchesQuery(res, q) {
				shown = append(shown, res)
			}
		}
		r.shown = shown
	}
	rows := rowsToTableRows(r.shown)
	if r.query == "" && m.hasMoreReviewed() {
		rows = append(rows, m.sentinelRow())
	}
	r.table.SetRows(rows)
	r.table.SetColumns(m.columnsFor(r.shown))
}

// sentinelRow builds the "Load more" row shown beneath the loaded results.
func (m *Model) sentinelRow() table.Row {
	label := "⬇  Load more"
	if m.reviewed.loading {
		label = "⏳ Loading…"
	}
	sentinel := make(table.Row, len(columnHeaders))
	sentinel[0] = label
	sentinel[len(sentinel)-1] = fmt.Sprintf("%d more", len(m.reviewed.urls)-m.reviewed.loaded)
	return sentinel
}

// matchesQuery reports whether a result's URL or title contains q (already
// lowercased).
func matchesQuery(r data.Result, q string) bool {
	if strings.Contains(strings.ToLower(r.URL), q) {
		return true
	}
	return r.Err == nil && strings.Contains(strings.ToLower(r.PR.Title), q)
}

func (m *Model) handleReviewedInit(msg reviewedInitMsg) {
	m.reviewed.loading = false
	if msg.err != nil {
		m.reviewed.err = msg.err.Error()
		return
	}
	m.reviewed.err = ""
	m.reviewed.urls = msg.urls
	m.reviewed.rows = msg.results
	m.reviewed.loaded = len(msg.results)
	m.refreshReviewedTable()
}

func (m *Model) handleReviewedPage(msg reviewedPageMsg) {
	m.reviewed.loading = false
	if msg.err != nil {
		m.reviewed.err = msg.err.Error()
		return
	}
	m.reviewed.err = ""
	m.reviewed.rows = append(m.reviewed.rows, msg.results...)
	m.reviewed.loaded = len(m.reviewed.rows)
	m.refreshReviewedTable()
}

func (m *Model) openPalette() *Model {
	m.palette = true
	m.paletteInput = ""
	m.overlay = ""
	m.err = ""
	m.status = "Command palette — type add <url>, usage, archive · esc cancels"
	return m
}

func (m *Model) handleEsc() (tea.Model, tea.Cmd, bool) {
	if m.overlay != "" {
		m.overlay = ""
		m.status = "Overlay closed"
		return m, nil, true
	}
	if m.tab == tabDone && m.reviewed.query != "" {
		m.reviewed.query = ""
		m.reviewed.table.SetCursor(0)
		m.refreshReviewedTable()
		return m, nil, true
	}
	model, cmd := m.quitOrPromptArchiveCore()
	return model, cmd, true
}

func (m *Model) quitOrPromptArchive() (tea.Model, tea.Cmd, bool) {
	model, cmd := m.quitOrPromptArchiveCore()
	return model, cmd, true
}

func (m *Model) quitOrPromptArchiveCore() (tea.Model, tea.Cmd) {
	terminal := m.terminalURLs()
	if len(terminal) == 0 {
		return m, tea.Quit
	}
	m.confirmArchive = true
	m.pendingArchive = terminal
	return m, nil
}

func (m *Model) refresh() (tea.Model, tea.Cmd, bool) {
	if m.loading {
		return m, nil, false
	}
	m.loading = true
	m.status = "Refreshing…"
	return m, tea.Batch(m.spinner.Tick, fetchActiveCmd(m.store, m.client)), true
}

func (m *Model) copySelectedURL() {
	url := m.selectedURL()
	if url == "" {
		return
	}
	if err := copyToClipboard(url); err != nil {
		m.err = err.Error()
		return
	}
	m.status = "Copied " + url
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
	b.WriteString(m.renderHeader())
	b.WriteString("\n\n")
	b.WriteString(m.renderTabs())
	b.WriteString("\n\n")

	if m.tab == tabDone {
		m.renderReviewedBody(&b)
	} else {
		m.renderActiveBody(&b)
	}

	if m.overlay != "" {
		b.WriteString("\n")
		b.WriteString(m.overlay)
	}

	switch {
	case m.confirmArchive:
		prompt := fmt.Sprintf("\n📦 Archive %d closed/merged PR(s)? [Y/n]", len(m.pendingArchive))
		b.WriteString(confirmStyle.Render(prompt))
	case m.confirmDelete:
		prompt := fmt.Sprintf("\n🗑  Delete %s? [y/N]", m.pendingDelete)
		b.WriteString(confirmStyle.Render(prompt))
	case m.palette:
		ghost := paletteSuggestion(m.paletteCommands(), m.paletteInput)
		b.WriteString("\n" + confirmStyle.Render("/"+m.paletteInput) + hintStyle.Render(ghost) + confirmStyle.Render("▌"))
		switch {
		case ghost != "":
			b.WriteString(hintStyle.Render("   (tab to complete · esc cancels)"))
		case m.tab == tabDone:
			b.WriteString(hintStyle.Render("   (command: search <keyword> · tab completes · esc cancels)"))
		default:
			b.WriteString(hintStyle.Render("   (commands: add <url>, usage, archive · tab completes · esc cancels)"))
		}
	default:
		b.WriteString("\n" + m.hintsLine())
	}
	return b.String()
}

// renderHeader draws the title badge followed by the current tab's status,
// separated by a faint divider.
func (m *Model) renderHeader() string {
	var b strings.Builder
	b.WriteString(titleStyle.Render("🦉 prowl"))
	if st := m.currentStatus(); st != "" {
		b.WriteString("  " + dividerStyle.Render("│") + "  ")
		if m.loading || m.reviewed.loading {
			b.WriteString(m.spinner.View() + " ")
		}
		b.WriteString(statusStyle.Render(st))
	}
	return b.String()
}

// renderTabs draws the tab bar as pills, the active one filled.
func (m *Model) renderTabs() string {
	parts := make([]string, len(tabTitles))
	for i, t := range tabTitles {
		if tabID(i) == m.tab {
			parts[i] = activeTab.Render(t)
		} else {
			parts[i] = inactiveTab.Render(t)
		}
	}
	return strings.Join(parts, " ")
}

// currentStatus is the status line for the active tab.
func (m *Model) currentStatus() string {
	if m.tab == tabDone {
		if m.reviewed.loading && len(m.reviewed.rows) == 0 {
			return "Loading done…"
		}
		if m.reviewed.query != "" {
			return fmt.Sprintf("search %q — %d/%d loaded · esc clears", m.reviewed.query, len(m.reviewed.shown), m.reviewed.loaded)
		}
		return fmt.Sprintf("done %d/%d (newest first)", m.reviewed.loaded, len(m.reviewed.urls))
	}
	return m.status
}

func (m *Model) renderActiveBody(b *strings.Builder) {
	if m.err != "" {
		b.WriteString(errStyle.Render("✗ " + m.err))
		b.WriteString("\n\n")
	}
	if len(m.rows) == 0 && !m.loading {
		b.WriteString(okStyle.Render("📭 no active PRs — `prowl add <url>` or press `/` then `add <url>`\n"))
		return
	}
	b.WriteString(m.table.View())
	b.WriteString("\n")
}

func (m *Model) renderReviewedBody(b *strings.Builder) {
	if m.reviewed.err != "" {
		b.WriteString(errStyle.Render("✗ " + m.reviewed.err))
		b.WriteString("\n\n")
	}
	if len(m.reviewed.shown) == 0 && !m.reviewed.loading {
		if m.reviewed.query != "" {
			b.WriteString(okStyle.Render(fmt.Sprintf("🔍 no matches for %q — esc to clear\n", m.reviewed.query)))
		} else {
			b.WriteString(okStyle.Render("📭 no done PRs yet — archive closed/merged PRs to see them here\n"))
		}
		return
	}
	b.WriteString(m.reviewed.table.View())
	b.WriteString("\n")
}

// hintsLine returns the key-hint footer for the active tab.
func (m *Model) hintsLine() string {
	nav := keyStyle.Render("↑↓/jk") + hintStyle.Render(" nav")
	tabs := keyStyle.Render("←→/hl") + hintStyle.Render(" tabs")
	open := keyStyle.Render("⏎") + hintStyle.Render(" open")
	copyHint := keyStyle.Render("c") + hintStyle.Render(" copy")
	quit := keyStyle.Render("q") + hintStyle.Render(" quit")
	var hints []string
	if m.tab == tabDone {
		search := keyStyle.Render("/") + hintStyle.Render(" search")
		hints = []string{nav, tabs, open, copyHint, search}
		if m.reviewed.query != "" {
			hints = append(hints, keyStyle.Render("esc")+hintStyle.Render(" clear"))
		}
		hints = append(hints, quit)
	} else {
		hints = []string{
			nav, tabs, open, copyHint,
			keyStyle.Render("d") + hintStyle.Render(" delete"),
			keyStyle.Render("r") + hintStyle.Render(" refresh"),
			keyStyle.Render("/") + hintStyle.Render(" cmd"),
			quit,
		}
	}
	return strings.Join(hints, hintStyle.Render(" · "))
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
	tbl, rows := m.table, m.rows
	if m.tab == tabDone {
		tbl, rows = m.reviewed.table, m.reviewed.shown
	}
	cursor := tbl.Cursor()
	if cursor < 0 || cursor >= len(rows) {
		return ""
	}
	return rows[cursor].URL
}

// resultCells returns the raw column values for a single result, in the order
// declared by columnHeaders.
func resultCells(r data.Result) []string {
	if r.Err != nil {
		return []string{data.ShortURL(r.URL), "-", statusEmojiLabel("error"), "-"}
	}
	pr := r.PR
	return []string{
		data.ShortURL(pr.URL),
		data.AssigneesLabel(pr),
		statusEmojiLabel(data.StatusLabel(pr)),
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
