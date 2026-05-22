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

// queueEmojiLabel decorates the queue column with a small visual cue.
func queueEmojiLabel(label string) string {
	if label == "-" {
		return label
	}
	return "🚦 " + label
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

func tableColumns() []table.Column {
	return []table.Column{
		{Title: "PR", Width: 7},
		{Title: "Assignee", Width: 18},
		{Title: "Status", Width: 16},
		{Title: "Queue", Width: 28},
		{Title: "Pos", Width: 4},
		{Title: "ETA", Width: 6},
		{Title: "URL", Width: 50},
	}
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
	case tea.KeyMsg:
		if m.confirmArchive {
			switch msg.String() {
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
		if m.confirmDelete {
			switch msg.String() {
			case "y", "Y":
				url := m.pendingDelete
				m.confirmDelete = false
				m.pendingDelete = ""
				if _, err := m.store.Remove(url); err != nil {
					m.err = err.Error()
					return m, nil
				}
				m.status = "Removed " + url
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
		switch msg.String() {
		case "q", "ctrl+c", "esc":
			terminal := m.terminalURLs()
			if len(terminal) == 0 {
				return m, tea.Quit
			}
			m.confirmArchive = true
			m.pendingArchive = terminal
			return m, nil
		case "r", "ctrl+r":
			if !m.loading {
				m.loading = true
				m.status = "Refreshing…"
				return m, tea.Batch(m.spinner.Tick, fetchActiveCmd(m.store, m.client))
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
				return m, nil
			}
		}
	case rowsReadyMsg:
		m.loading = false
		if msg.err != nil {
			m.err = msg.err.Error()
			m.rows = nil
			m.table.SetRows(nil)
			return m, nil
		}
		m.err = ""
		m.rows = msg.results
		m.table.SetRows(rowsToTableRows(msg.results))
		m.status = summary(msg.results)
	case autoRefreshTickMsg:
		// Re-arm the tick first so the cadence stays steady even if a fetch
		// is already in flight. Skip the fetch when one is in flight, when
		// the user is in a confirmation prompt, or when auto-refresh was
		// disabled at runtime.
		next := m.autoRefreshCmd()
		if next == nil {
			return m, nil
		}
		if m.loading || m.confirmArchive || m.confirmDelete {
			return m, next
		}
		m.loading = true
		m.status = "Auto-refreshing…"
		return m, tea.Batch(m.spinner.Tick, fetchActiveCmd(m.store, m.client), next)
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
		b.WriteString(okStyle.Render("📭 no active PRs — `prowl add <url>` to track one\n"))
	} else {
		b.WriteString(m.table.View())
		b.WriteString("\n")
	}

	switch {
	case m.confirmArchive:
		prompt := fmt.Sprintf("\n📦 Archive %d closed/merged PR(s)? [y/N]", len(m.pendingArchive))
		b.WriteString(confirmStyle.Render(prompt))
	case m.confirmDelete:
		prompt := fmt.Sprintf("\n🗑  Delete %s? [y/N]", m.pendingDelete)
		b.WriteString(confirmStyle.Render(prompt))
	default:
		hints := []string{
			keyStyle.Render("↑↓") + hintStyle.Render(" nav"),
			keyStyle.Render("⏎") + hintStyle.Render(" open"),
			keyStyle.Render("c") + hintStyle.Render(" copy"),
			keyStyle.Render("d") + hintStyle.Render(" delete"),
			keyStyle.Render("r") + hintStyle.Render(" refresh"),
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
	row := m.table.SelectedRow()
	if len(row) < 7 {
		return ""
	}
	return row[6]
}

func rowsToTableRows(results []data.Result) []table.Row {
	out := make([]table.Row, 0, len(results))
	for _, r := range results {
		if r.Err != nil {
			out = append(out, table.Row{"?", "-", statusEmojiLabel("error"), "-", "-", "-", r.URL})
			continue
		}
		pr := r.PR
		num := "?"
		if pr.Number > 0 {
			num = fmt.Sprintf("#%d", pr.Number)
		}
		out = append(out, table.Row{
			num,
			data.AssigneesLabel(pr),
			statusEmojiLabel(data.StatusLabel(pr)),
			queueEmojiLabel(data.QueueLabel(pr)),
			data.QueuePositionLabel(pr),
			data.ETALabel(pr),
			pr.URL,
		})
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
