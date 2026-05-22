package cmd

import "github.com/figarocorso/prowl/internal/tui"

func runTUI() error {
	return tui.Run(dataDir, profile)
}
