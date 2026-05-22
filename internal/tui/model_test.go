package tui

import (
	"context"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/exp/teatest"
	"github.com/figarocorso/prowl/internal/config"
	"github.com/figarocorso/prowl/internal/data"
	"github.com/figarocorso/prowl/internal/store"
	"github.com/stretchr/testify/require"
)

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
		return strings.Contains(string(b), "#1234")
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
		if strings.Contains(string(b), "#1234") {
			captured = append([]byte(nil), b...)
			return true
		}
		return false
	}, teatest.WithCheckInterval(20*time.Millisecond), teatest.WithDuration(2*time.Second))

	tm.Send(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'q'}})
	tm.WaitFinished(t, teatest.WithFinalTimeout(2*time.Second))

	out := stripANSI(string(captured))
	// Find the line containing "PR" header. It must NOT have side-border glyphs.
	var headerLine string
	for _, ln := range strings.Split(out, "\n") {
		trim := strings.TrimSpace(ln)
		if strings.HasPrefix(trim, "PR") && strings.Contains(ln, "Assignee") {
			headerLine = ln
			break
		}
	}
	require.NotEmpty(t, headerLine, "header line not found in output:\n%s", out)
	require.NotContains(t, headerLine, "│", "header should have no vertical border glyphs")

	// PR header column start must equal data column start.
	prHeaderCol := strings.Index(headerLine, "PR")
	var dataLine string
	for _, ln := range strings.Split(out, "\n") {
		if strings.Contains(ln, "#1234") {
			dataLine = ln
			break
		}
	}
	require.NotEmpty(t, dataLine)
	prDataCol := strings.Index(dataLine, "#1234")
	require.Equal(t, prHeaderCol, prDataCol, "PR header and row must start at same column\nheader: %q\nrow:    %q", headerLine, dataLine)
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
		return strings.Contains(string(b), "#1234")
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
		return strings.Contains(string(b), "#1234")
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
		return strings.Contains(string(b), "#1234")
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
