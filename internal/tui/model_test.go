package tui

import (
	"context"
	"errors"
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
		if strings.HasPrefix(trim, "URL") && strings.Contains(ln, "Status") {
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

	// Wait for the refetch summary to land — exactly one open PR (1235) remains.
	teatest.WaitFor(t, tm.Output(), func(b []byte) bool {
		return strings.Contains(string(b), "1 open")
	}, teatest.WithCheckInterval(20*time.Millisecond), teatest.WithDuration(2*time.Second))

	tm.Send(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'q'}})
	tm.WaitFinished(t, teatest.WithFinalTimeout(2*time.Second))

	active, err := m.store.Active()
	require.NoError(t, err)
	require.Equal(t, []string{kept}, active)
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

// TestVimNavigation locks in j/k moving the table cursor down/up, matching the
// arrow keys advertised in the hint bar.
func TestVimNavigation(t *testing.T) {
	m := newTestModel(t, nil)
	m.handleRowsReady(rowsReadyMsg{results: []data.Result{
		{URL: "https://github.com/acme/api/pull/1", PR: data.PR{URL: "https://github.com/acme/api/pull/1", State: "OPEN", Title: "first"}},
		{URL: "https://github.com/acme/api/pull/2", PR: data.PR{URL: "https://github.com/acme/api/pull/2", State: "OPEN", Title: "second"}},
		{URL: "https://github.com/acme/api/pull/3", PR: data.PR{URL: "https://github.com/acme/api/pull/3", State: "OPEN", Title: "third"}},
	}})

	require.Equal(t, 0, m.table.Cursor())

	sendKey := func(r rune) {
		updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}})
		m = updated.(*Model)
	}

	sendKey('j')
	require.Equal(t, 1, m.table.Cursor(), "j should move cursor down")
	sendKey('j')
	require.Equal(t, 2, m.table.Cursor(), "j should move cursor down again")
	sendKey('k')
	require.Equal(t, 1, m.table.Cursor(), "k should move cursor up")
}

// --- Done tab: pagination + search ---------------------------------------

func doneTestModel(t *testing.T) *Model {
	t.Helper()
	m := newTestModel(t, nil)
	m.tab = tabDone
	m.width = 120
	m.height = 30
	return m
}

func TestMatchesQuery(t *testing.T) {
	r := data.Result{
		URL: "https://github.com/acme/api/pull/1235",
		PR:  data.PR{URL: "https://github.com/acme/api/pull/1235", Title: "Refactor auth middleware"},
	}
	require.True(t, matchesQuery(r, "auth"))  // title
	require.True(t, matchesQuery(r, "1235"))  // url
	require.False(t, matchesQuery(r, "zzzz")) // no match
	bad := data.Result{URL: "https://github.com/acme/api/pull/9", Err: errors.New("boom")}
	require.True(t, matchesQuery(bad, "pull/9")) // url still matches on error rows
	require.False(t, matchesQuery(bad, "auth"))  // no title to match
}

func TestPageSize(t *testing.T) {
	m := doneTestModel(t)
	m.height = 30
	require.Equal(t, 20, m.pageSize()) // capped at 20
	m.height = 15
	require.Equal(t, 6, m.pageSize()) // visibleRows 9 - 3
	m.height = 0
	require.Equal(t, 2, m.pageSize()) // visibleRows floor 5 - 3
}

func TestSwitchTabInitializesDone(t *testing.T) {
	m := newTestModel(t, nil)
	_, cmd, handled := m.switchTab(tabDone)
	require.True(t, handled)
	require.NotNil(t, cmd, "first switch to Done kicks off a fetch")
	require.True(t, m.reviewed.initialized)
	require.Equal(t, tabDone, m.tab)

	_, cmd, _ = m.switchTab(tabDone) // same tab is a no-op
	require.Nil(t, cmd)

	_, cmd, _ = m.switchTab(tabActive)
	require.Nil(t, cmd)
	require.Equal(t, tabActive, m.tab)
}

func TestReviewedPaginationSentinelAndLoadMore(t *testing.T) {
	m := doneTestModel(t)
	ctx := context.Background()
	urls := []string{
		"https://github.com/acme/api/pull/1234",
		"https://github.com/acme/api/pull/1235",
		"https://github.com/acme/api/pull/1198",
		"https://github.com/acme/api/pull/1199",
		"https://github.com/acme/api/pull/1200",
	}

	first := m.client.FetchBatch(ctx, urls[:3])
	m.handleReviewedInit(reviewedInitMsg{urls: urls, results: first})
	require.Equal(t, 3, m.reviewed.loaded)
	require.True(t, m.hasMoreReviewed())
	require.Len(t, m.reviewed.table.Rows(), 4, "3 rows + Load more sentinel")

	m.reviewed.table.SetCursor(3) // sentinel row
	require.True(t, m.loadMoreSelected())
	cmd := m.loadMoreReviewed()
	require.NotNil(t, cmd)
	require.True(t, m.reviewed.loading)

	// Drive the page fetch the way the sentinel's command would.
	pageMsg := fetchReviewedPageCmd(m.client, urls[3:])().(reviewedPageMsg)
	m.handleReviewedPage(pageMsg)
	require.Equal(t, 5, m.reviewed.loaded)
	require.False(t, m.hasMoreReviewed())
	require.Len(t, m.reviewed.table.Rows(), 5, "all loaded, sentinel gone")
}

func TestReviewedInitError(t *testing.T) {
	m := doneTestModel(t)
	m.handleReviewedInit(reviewedInitMsg{err: errors.New("nope")})
	require.Equal(t, "nope", m.reviewed.err)
	require.False(t, m.reviewed.loading)
}

func TestSearchFiltersLoadedRowsAndEscClears(t *testing.T) {
	m := doneTestModel(t)
	ctx := context.Background()
	urls := []string{
		"https://github.com/acme/api/pull/1234", // Add /healthz endpoint
		"https://github.com/acme/api/pull/1235", // Refactor auth middleware
	}
	m.handleReviewedInit(reviewedInitMsg{urls: urls, results: m.client.FetchBatch(ctx, urls)})
	require.Len(t, m.reviewed.shown, 2)

	// Search dispatched through the palette to cover the command router too.
	m.runPaletteCommand("search AUTH") // case-insensitive
	require.Equal(t, "AUTH", m.reviewed.query)
	require.Len(t, m.reviewed.shown, 1)
	require.Contains(t, m.reviewed.shown[0].URL, "1235")

	_, _, handled := m.handleEsc() // clears the search
	require.True(t, handled)
	require.Empty(t, m.reviewed.query)
	require.Len(t, m.reviewed.shown, 2)
}

func TestRunPaletteSearchGuards(t *testing.T) {
	m := newTestModel(t, nil) // Active tab
	m.runPaletteCommand("search foo")
	require.Contains(t, m.err, "only available on the Done tab")

	m.tab = tabDone
	m.err = ""
	m.runPaletteSearch(nil)
	require.Contains(t, m.err, "usage")
}

func TestHandleEnterOnSentinelLoadsMore(t *testing.T) {
	m := doneTestModel(t)
	urls := []string{
		"https://github.com/acme/api/pull/1234",
		"https://github.com/acme/api/pull/1235",
	}
	m.handleReviewedInit(reviewedInitMsg{urls: urls, results: m.client.FetchBatch(context.Background(), urls[:1])})
	m.reviewed.table.SetCursor(1) // sentinel
	_, cmd, handled := m.handleEnter()
	require.True(t, handled)
	require.NotNil(t, cmd)
}

func TestFetchReviewedInitCmdReversesNewestFirst(t *testing.T) {
	dir := t.TempDir()
	s, err := store.New(filepath.Join(dir, "a.txt"), filepath.Join(dir, "r.txt"))
	require.NoError(t, err)
	urls := []string{
		"https://github.com/acme/api/pull/1198",
		"https://github.com/acme/api/pull/1199",
		"https://github.com/acme/api/pull/1200",
	}
	for _, u := range urls {
		_, err := s.Add(u)
		require.NoError(t, err)
	}
	_, err = s.MoveActiveToReviewed(urls)
	require.NoError(t, err)

	mock := data.NewMockClient()
	require.NoError(t, mock.LoadFixtures(filepath.Join("..", "..", "internal", "data", "testdata", "fixtures.json")))

	msg := fetchReviewedInitCmd(s, mock, 2)().(reviewedInitMsg)
	require.NoError(t, msg.err)
	require.Equal(t, []string{
		"https://github.com/acme/api/pull/1200",
		"https://github.com/acme/api/pull/1199",
		"https://github.com/acme/api/pull/1198",
	}, msg.urls, "newest first")
	require.Len(t, msg.results, 2, "only the first page is fetched")
}
