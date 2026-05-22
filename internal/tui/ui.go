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
