package tui

import (
	"context"
	"path/filepath"
	"regexp"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/exp/teatest"
	"github.com/figarocorso/prowl/internal/config"
	"github.com/figarocorso/prowl/internal/data"
	"github.com/figarocorso/prowl/internal/store"
	"github.com/stretchr/testify/require"
)

// countingClient wraps a PRClient and counts FetchBatch invocations.
type countingClient struct {
	inner data.PRClient
	calls atomic.Int32
}

func (c *countingClient) Fetch(ctx context.Context, url string) (data.PR, error) {
	return c.inner.Fetch(ctx, url)
}

func (c *countingClient) FetchBatch(ctx context.Context, urls []string) []data.Result {
	c.calls.Add(1)
	return c.inner.FetchBatch(ctx, urls)
}

var ansiRE = regexp.MustCompile(`\x1b\[[0-9;?]*[a-zA-Z]`)

func stripANSI(s string) string { return ansiRE.ReplaceAllString(s, "") }

func newTestModel(t *testing.T, urls []string) *Model {
	t.Helper()
	dir := t.TempDir()
	s, err := store.New(filepath.Join(dir, "a.txt"), filepath.Join(dir, "r.txt"))
	require.NoError(t, err)
	for _, u := range urls {
		_, err := s.Add(u)
		require.NoError(t, err)
	}

	mock := data.NewMockClient()
	require.NoError(t, mock.LoadFixtures(filepath.Join("..", "..", "internal", "data", "testdata", "fixtures.json")))

	cfg := &config.Config{Paths: config.Paths{DataDir: dir}}
	return New(cfg, s, mock)
}

func TestModelRendersRows(t *testing.T) {
	m := newTestModel(t, []string{"https://github.com/acme/api/pull/1234"})
	tm := teatest.NewTestModel(t, m, teatest.WithInitialTermSize(120, 30))

	teatest.WaitFor(t, tm.Output(), func(b []byte) bool {
		return strings.Contains(string(b), "pull/1234")
	}, teatest.WithCheckInterval(20*time.Millisecond), teatest.WithDuration(2*time.Second))

	tm.Send(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'q'}})
	tm.WaitFinished(t, teatest.WithFinalTimeout(2*time.Second))
}

func TestModelEmptyState(t *testing.T) {
	m := newTestModel(t, nil)
	tm := teatest.NewTestModel(t, m, teatest.WithInitialTermSize(120, 30))

	teatest.WaitFor(t, tm.Output(), func(b []byte) bool {
		return strings.Contains(string(b), "no active PRs")
	}, teatest.WithCheckInterval(20*time.Millisecond), teatest.WithDuration(2*time.Second))

	tm.Send(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'q'}})
	tm.WaitFinished(t, teatest.WithFinalTimeout(2*time.Second))
}

func TestModelHeaderHasNoSideBorders(t *testing.T) {
	m := newTestModel(t, []string{"https://github.com/acme/api/pull/1234"})
	tm := teatest.NewTestModel(t, m, teatest.WithInitialTermSize(200, 30))

	var captured []byte
	teatest.WaitFor(t, tm.Output(), func(b []byte) bool {
		if strings.Contains(string(b), "pull/1234") {
			captured = append([]byte(nil), b...)
			return true
		}
		return false
	}, teatest.WithCheckInterval(20*time.Millisecond), teatest.WithDuration(2*time.Second))

	tm.Send(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'q'}})
	tm.WaitFinished(t, teatest.WithFinalTimeout(2*time.Second))

	out := stripANSI(string(captured))
	// Find the line containing "URL" header. It must NOT have side-border glyphs.
	var headerLine string
	for _, ln := range strings.Split(out, "\n") {
		trim := strings.TrimSpace(ln)
		if strings.HasPrefix(trim, "URL") && strings.Contains(ln, "Assignee") {
			headerLine = ln
			break
		}
	}
	require.NotEmpty(t, headerLine, "header line not found in output:\n%s", out)
	require.NotContains(t, headerLine, "│", "header should have no vertical border glyphs")

	// URL header column start must equal data column start.
	urlHeaderCol := strings.Index(headerLine, "URL")
	var dataLine string
	for _, ln := range strings.Split(out, "\n") {
		if strings.Contains(ln, "pull/1234") {
			dataLine = ln
			break
		}
	}
	require.NotEmpty(t, dataLine)
	urlDataCol := strings.Index(dataLine, "acme/api/pull/1234")
	require.Equal(t, urlHeaderCol, urlDataCol, "URL header and row must start at same column\nheader: %q\nrow:    %q", headerLine, dataLine)
}

func TestQuitPromptsArchiveWhenTerminalPRs(t *testing.T) {
	urls := []string{
		"https://github.com/acme/api/pull/1234", // OPEN
		"https://github.com/acme/api/pull/1198", // MERGED
		"https://github.com/acme/api/pull/1200", // CLOSED
	}
	m := newTestModel(t, urls)
	tm := teatest.NewTestModel(t, m, teatest.WithInitialTermSize(200, 30))

	teatest.WaitFor(t, tm.Output(), func(b []byte) bool {
		return strings.Contains(string(b), "pull/1234")
	}, teatest.WithCheckInterval(20*time.Millisecond), teatest.WithDuration(2*time.Second))

	tm.Send(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'q'}})

	teatest.WaitFor(t, tm.Output(), func(b []byte) bool {
		return strings.Contains(string(b), "Archive 2 closed/merged PR(s)?")
	}, teatest.WithCheckInterval(20*time.Millisecond), teatest.WithDuration(2*time.Second))

	tm.Send(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'y'}})
	tm.WaitFinished(t, teatest.WithFinalTimeout(2*time.Second))

	active, err := m.store.Active()
	require.NoError(t, err)
	require.Equal(t, []string{"https://github.com/acme/api/pull/1234"}, active)

	reviewed, err := m.store.Reviewed()
	require.NoError(t, err)
	require.ElementsMatch(t, []string{
		"https://github.com/acme/api/pull/1198",
		"https://github.com/acme/api/pull/1200",
	}, reviewed)
}

func TestQuitImmediateWhenNoTerminalPRs(t *testing.T) {
	m := newTestModel(t, []string{"https://github.com/acme/api/pull/1234"})
	tm := teatest.NewTestModel(t, m, teatest.WithInitialTermSize(200, 30))

	teatest.WaitFor(t, tm.Output(), func(b []byte) bool {
		return strings.Contains(string(b), "pull/1234")
	}, teatest.WithCheckInterval(20*time.Millisecond), teatest.WithDuration(2*time.Second))

	tm.Send(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'q'}})
	tm.WaitFinished(t, teatest.WithFinalTimeout(2*time.Second))
}

func TestQuitArchivePromptDeclined(t *testing.T) {
	urls := []string{
		"https://github.com/acme/api/pull/1234",
		"https://github.com/acme/api/pull/1200",
	}
	m := newTestModel(t, urls)
	tm := teatest.NewTestModel(t, m, teatest.WithInitialTermSize(200, 30))

	teatest.WaitFor(t, tm.Output(), func(b []byte) bool {
		return strings.Contains(string(b), "pull/1234")
	}, teatest.WithCheckInterval(20*time.Millisecond), teatest.WithDuration(2*time.Second))

	tm.Send(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'q'}})

	teatest.WaitFor(t, tm.Output(), func(b []byte) bool {
		return strings.Contains(string(b), "Archive 1 closed/merged PR(s)?")
	}, teatest.WithCheckInterval(20*time.Millisecond), teatest.WithDuration(2*time.Second))

	tm.Send(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'n'}})
	tm.WaitFinished(t, teatest.WithFinalTimeout(2*time.Second))

	active, err := m.store.Active()
	require.NoError(t, err)
	require.ElementsMatch(t, urls, active)

	reviewed, err := m.store.Reviewed()
	require.NoError(t, err)
	require.Empty(t, reviewed)
}

func TestDeletePromptsConfirmation(t *testing.T) {
	url := "https://github.com/acme/api/pull/1234"
	m := newTestModel(t, []string{url})
	tm := teatest.NewTestModel(t, m, teatest.WithInitialTermSize(200, 30))

	teatest.WaitFor(t, tm.Output(), func(b []byte) bool {
		return strings.Contains(string(b), "pull/1234")
	}, teatest.WithCheckInterval(20*time.Millisecond), teatest.WithDuration(2*time.Second))

	tm.Send(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'d'}})

	teatest.WaitFor(t, tm.Output(), func(b []byte) bool {
		return strings.Contains(string(b), "Delete "+url+"?")
	}, teatest.WithCheckInterval(20*time.Millisecond), teatest.WithDuration(2*time.Second))

	tm.Send(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'y'}})

	teatest.WaitFor(t, tm.Output(), func(b []byte) bool {
		return strings.Contains(string(b), "no active PRs")
	}, teatest.WithCheckInterval(20*time.Millisecond), teatest.WithDuration(2*time.Second))

	tm.Send(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'q'}})
	tm.WaitFinished(t, teatest.WithFinalTimeout(2*time.Second))

	active, err := m.store.Active()
	require.NoError(t, err)
	require.Empty(t, active)
}

func TestDeleteRemovesRowImmediately(t *testing.T) {
	deleted := "https://github.com/acme/api/pull/1234"
	kept := "https://github.com/acme/api/pull/1235"
	m := newTestModel(t, []string{deleted, kept})
	tm := teatest.NewTestModel(t, m, teatest.WithInitialTermSize(200, 30))

	teatest.WaitFor(t, tm.Output(), func(b []byte) bool {
		return strings.Contains(string(b), "pull/1234") && strings.Contains(string(b), "pull/1235")
	}, teatest.WithCheckInterval(20*time.Millisecond), teatest.WithDuration(2*time.Second))

	tm.Send(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'d'}})
	teatest.WaitFor(t, tm.Output(), func(b []byte) bool {
		return strings.Contains(string(b), "Delete "+deleted+"?")
	}, teatest.WithCheckInterval(20*time.Millisecond), teatest.WithDuration(2*time.Second))

	tm.Send(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'y'}})

	teatest.WaitFor(t, tm.Output(), func(b []byte) bool {
		s := string(b)
		return !strings.Contains(s, "pull/1234") && strings.Contains(s, "pull/1235")
	}, teatest.WithCheckInterval(20*time.Millisecond), teatest.WithDuration(2*time.Second))

	tm.Send(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'q'}})
	tm.WaitFinished(t, teatest.WithFinalTimeout(2*time.Second))
}

func TestDeletePromptCancelKeepsRow(t *testing.T) {
	url := "https://github.com/acme/api/pull/1234"
	m := newTestModel(t, []string{url})
	tm := teatest.NewTestModel(t, m, teatest.WithInitialTermSize(200, 30))

	teatest.WaitFor(t, tm.Output(), func(b []byte) bool {
		return strings.Contains(string(b), "pull/1234")
	}, teatest.WithCheckInterval(20*time.Millisecond), teatest.WithDuration(2*time.Second))

	tm.Send(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'d'}})

	teatest.WaitFor(t, tm.Output(), func(b []byte) bool {
		return strings.Contains(string(b), "Delete "+url+"?")
	}, teatest.WithCheckInterval(20*time.Millisecond), teatest.WithDuration(2*time.Second))

	tm.Send(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'n'}})

	teatest.WaitFor(t, tm.Output(), func(b []byte) bool {
		return strings.Contains(string(b), "Delete cancelled")
	}, teatest.WithCheckInterval(20*time.Millisecond), teatest.WithDuration(2*time.Second))

	tm.Send(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'q'}})
	tm.WaitFinished(t, teatest.WithFinalTimeout(2*time.Second))

	active, err := m.store.Active()
	require.NoError(t, err)
	require.Equal(t, []string{url}, active)
}

func TestAutoRefreshTickRefetches(t *testing.T) {
	url := "https://github.com/acme/api/pull/1234"

	dir := t.TempDir()
	s, err := store.New(filepath.Join(dir, "a.txt"), filepath.Join(dir, "r.txt"))
	require.NoError(t, err)
	_, err = s.Add(url)
	require.NoError(t, err)

	mock := data.NewMockClient()
	require.NoError(t, mock.LoadFixtures(filepath.Join("..", "..", "internal", "data", "testdata", "fixtures.json")))
	counter := &countingClient{inner: mock}

	cfg := &config.Config{Paths: config.Paths{DataDir: dir}}
	m := New(cfg, s, counter)
	m.SetRefreshInterval(50 * time.Millisecond)

	tm := teatest.NewTestModel(t, m, teatest.WithInitialTermSize(200, 30))

	// Wait for the initial fetch to render.
	teatest.WaitFor(t, tm.Output(), func(b []byte) bool {
		return strings.Contains(string(b), "pull/1234")
	}, teatest.WithCheckInterval(20*time.Millisecond), teatest.WithDuration(2*time.Second))
	require.GreaterOrEqual(t, counter.calls.Load(), int32(1))

	// Wait for at least one auto-refresh tick to drive a second FetchBatch.
	require.Eventually(t, func() bool {
		return counter.calls.Load() >= 2
	}, 2*time.Second, 20*time.Millisecond)

	tm.Send(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'q'}})
	tm.WaitFinished(t, teatest.WithFinalTimeout(2*time.Second))
}

func TestAutoRefreshDisabledWhenIntervalZero(t *testing.T) {
	m := newTestModel(t, nil)
	// No SetRefreshInterval call — default zero means disabled.
	require.Nil(t, m.autoRefreshCmd(), "auto refresh cmd should be nil when interval is 0")

	m.SetRefreshInterval(-1)
	require.Nil(t, m.autoRefreshCmd(), "auto refresh cmd should be nil for non-positive interval")

	m.SetRefreshInterval(10 * time.Second)
	require.NotNil(t, m.autoRefreshCmd(), "auto refresh cmd should be set when interval is positive")
}

func sendString(tm *teatest.TestModel, s string) {
	for _, r := range s {
		if r == ' ' {
			tm.Send(tea.KeyMsg{Type: tea.KeySpace, Runes: []rune{' '}})
			continue
		}
		tm.Send(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}})
	}
}

func TestPaletteAddAddsPR(t *testing.T) {
	m := newTestModel(t, nil)
	tm := teatest.NewTestModel(t, m, teatest.WithInitialTermSize(200, 30))

	teatest.WaitFor(t, tm.Output(), func(b []byte) bool {
		return strings.Contains(string(b), "no active PRs")
	}, teatest.WithCheckInterval(20*time.Millisecond), teatest.WithDuration(2*time.Second))

	tm.Send(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'/'}})
	sendString(tm, "add https://github.com/acme/api/pull/1234")
	tm.Send(tea.KeyMsg{Type: tea.KeyEnter})

	teatest.WaitFor(t, tm.Output(), func(b []byte) bool {
		return strings.Contains(string(b), "pull/1234")
	}, teatest.WithCheckInterval(20*time.Millisecond), teatest.WithDuration(2*time.Second))

	tm.Send(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'q'}})
	tm.WaitFinished(t, teatest.WithFinalTimeout(2*time.Second))

	active, err := m.store.Active()
	require.NoError(t, err)
	require.Equal(t, []string{"https://github.com/acme/api/pull/1234"}, active)
}

func TestPaletteAddAcceptsPaste(t *testing.T) {
	m := newTestModel(t, nil)
	tm := teatest.NewTestModel(t, m, teatest.WithInitialTermSize(200, 30))

	teatest.WaitFor(t, tm.Output(), func(b []byte) bool {
		return strings.Contains(string(b), "no active PRs")
	}, teatest.WithCheckInterval(20*time.Millisecond), teatest.WithDuration(2*time.Second))

	tm.Send(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'/'}})
	tm.Send(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("add ")})
	tm.Send(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("https://github.com/acme/api/pull/1234")})
	tm.Send(tea.KeyMsg{Type: tea.KeyEnter})

	teatest.WaitFor(t, tm.Output(), func(b []byte) bool {
		return strings.Contains(string(b), "pull/1234")
	}, teatest.WithCheckInterval(20*time.Millisecond), teatest.WithDuration(2*time.Second))

	tm.Send(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'q'}})
	tm.WaitFinished(t, teatest.WithFinalTimeout(2*time.Second))

	active, err := m.store.Active()
	require.NoError(t, err)
	require.Equal(t, []string{"https://github.com/acme/api/pull/1234"}, active)
}

func TestPaletteArchiveMovesTerminal(t *testing.T) {
	urls := []string{
		"https://github.com/acme/api/pull/1234",
		"https://github.com/acme/api/pull/1198",
	}
	m := newTestModel(t, urls)
	tm := teatest.NewTestModel(t, m, teatest.WithInitialTermSize(200, 30))

	teatest.WaitFor(t, tm.Output(), func(b []byte) bool {
		return strings.Contains(string(b), "pull/1234")
	}, teatest.WithCheckInterval(20*time.Millisecond), teatest.WithDuration(2*time.Second))

	tm.Send(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'/'}})
	sendString(tm, "archive")
	tm.Send(tea.KeyMsg{Type: tea.KeyEnter})

	teatest.WaitFor(t, tm.Output(), func(b []byte) bool {
		return strings.Contains(string(b), "0 merged")
	}, teatest.WithCheckInterval(20*time.Millisecond), teatest.WithDuration(3*time.Second))

	tm.Send(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'q'}})
	tm.WaitFinished(t, teatest.WithFinalTimeout(2*time.Second))

	active, err := m.store.Active()
	require.NoError(t, err)
	require.Equal(t, []string{"https://github.com/acme/api/pull/1234"}, active)
	reviewed, err := m.store.Reviewed()
	require.NoError(t, err)
	require.Equal(t, []string{"https://github.com/acme/api/pull/1198"}, reviewed)
}

func TestPaletteUsageShowsOverlay(t *testing.T) {
	m := newTestModel(t, []string{"https://github.com/acme/api/pull/1234"})
	tm := teatest.NewTestModel(t, m, teatest.WithInitialTermSize(200, 30))

	teatest.WaitFor(t, tm.Output(), func(b []byte) bool {
		return strings.Contains(string(b), "pull/1234")
	}, teatest.WithCheckInterval(20*time.Millisecond), teatest.WithDuration(3*time.Second))

	tm.Send(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'/'}})
	sendString(tm, "usage")
	tm.Send(tea.KeyMsg{Type: tea.KeyEnter})

	teatest.WaitFor(t, tm.Output(), func(b []byte) bool {
		return strings.Contains(string(b), "prowl usage")
	}, teatest.WithCheckInterval(20*time.Millisecond), teatest.WithDuration(3*time.Second))

	require.NotEmpty(t, m.overlay)
	require.Contains(t, m.overlay, "tracked total")

	tm.Send(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'q'}})
	tm.WaitFinished(t, teatest.WithFinalTimeout(2*time.Second))
}

func TestModelSummary(t *testing.T) {
	mock := data.NewMockClient()
	require.NoError(t, mock.LoadFixtures(filepath.Join("..", "..", "internal", "data", "testdata", "fixtures.json")))
	results := mock.FetchBatch(context.Background(), []string{
		"https://github.com/acme/api/pull/1234",
		"https://github.com/acme/api/pull/1198",
		"https://github.com/acme/api/pull/1200",
	})
	s := summary(results)
	require.Contains(t, s, "1 open")
	require.Contains(t, s, "1 merged")
	require.Contains(t, s, "1 closed")
}
