// Package tui hosts the Bubble Tea TUI for prowl.
package tui

import (
	"context"
	"fmt"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/figarocorso/prowl/internal/config"
	"github.com/figarocorso/prowl/internal/data"
	"github.com/figarocorso/prowl/internal/store"
)

// Run boots the TUI against the real config + store + GitHub client.
func Run(dataDir, profile string) error {
	return run(dataDir, profile, 0)
}

// RunWatch behaves like Run but auto-refreshes the PR list every interval.
// A non-positive interval disables auto-refresh (same as Run).
func RunWatch(dataDir, profile string, interval time.Duration) error {
	return run(dataDir, profile, interval)
}

func run(dataDir, profile string, interval time.Duration) error {
	cfg, err := config.Load(dataDir, profile)
	if err != nil {
		return fmt.Errorf("load config: %w", err)
	}
	s, err := store.New(cfg.Paths.ActiveFile, cfg.Paths.ReviewedFile)
	if err != nil {
		return fmt.Errorf("init store: %w", err)
	}
	client, err := data.NewGHClient()
	if err != nil {
		return fmt.Errorf("github client: %w", err)
	}
	m := New(cfg, s, client)
	m.SetRefreshInterval(interval)
	p := tea.NewProgram(m, tea.WithAltScreen())
	_, err = p.Run()
	return err
}

// rowsReadyMsg is delivered when a background fetch completes.
type rowsReadyMsg struct {
	results []data.Result
	err     error
}

// fetchActiveCmd returns a Bubble Tea command that fetches the active list.
func fetchActiveCmd(s *store.Store, client data.PRClient) tea.Cmd {
	return func() tea.Msg {
		urls, err := s.Active()
		if err != nil {
			return rowsReadyMsg{err: err}
		}
		if len(urls) == 0 {
			return rowsReadyMsg{}
		}
		return rowsReadyMsg{results: client.FetchBatch(context.Background(), urls)}
	}
}

// reviewedInitMsg carries the full reviewed URL list (newest-first) plus the
// first fetched page.
type reviewedInitMsg struct {
	urls    []string
	results []data.Result
	err     error
}

// reviewedPageMsg carries one freshly fetched page to append to the reviewed
// list.
type reviewedPageMsg struct {
	results []data.Result
	err     error
}

// fetchReviewedInitCmd reads the reviewed list, reverses it to newest-first,
// and fetches the first page (up to size URLs).
func fetchReviewedInitCmd(s *store.Store, client data.PRClient, size int) tea.Cmd {
	return func() tea.Msg {
		urls, err := s.Reviewed()
		if err != nil {
			return reviewedInitMsg{err: err}
		}
		urls = reverseURLs(urls)
		page := urls
		if len(page) > size {
			page = page[:size]
		}
		var results []data.Result
		if len(page) > 0 {
			results = client.FetchBatch(context.Background(), page)
		}
		return reviewedInitMsg{urls: urls, results: results}
	}
}

// fetchReviewedPageCmd fetches a specific slice of reviewed URLs.
func fetchReviewedPageCmd(client data.PRClient, urls []string) tea.Cmd {
	return func() tea.Msg {
		return reviewedPageMsg{results: client.FetchBatch(context.Background(), urls)}
	}
}

// reverseURLs returns a new slice with the elements in reverse order so the
// most recently archived PRs come first.
func reverseURLs(in []string) []string {
	out := make([]string, len(in))
	for i, v := range in {
		out[len(in)-1-i] = v
	}
	return out
}
