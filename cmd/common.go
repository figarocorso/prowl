package cmd

import (
	"fmt"

	"github.com/figarocorso/prowl/internal/config"
	"github.com/figarocorso/prowl/internal/data"
	"github.com/figarocorso/prowl/internal/store"
)

// clientFactory builds a PRClient. Overridden in tests.
var clientFactory = func() (data.PRClient, error) {
	return data.NewGHClient()
}

func loadConfigAndStore() (*config.Config, *store.Store, error) {
	cfg, err := config.Load(dataDir)
	if err != nil {
		return nil, nil, fmt.Errorf("load config: %w", err)
	}
	s, err := store.New(cfg.Paths.ActiveFile, cfg.Paths.ReviewedFile)
	if err != nil {
		return nil, nil, fmt.Errorf("init store: %w", err)
	}
	return cfg, s, nil
}
