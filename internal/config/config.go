package config

import (
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/knadh/koanf/parsers/yaml"
	"github.com/knadh/koanf/providers/file"
	"github.com/knadh/koanf/v2"
)

const (
	defaultRefreshInterval = 30 * time.Second
)

// Paths holds resolved on-disk locations for prowl's state.
type Paths struct {
	DataDir      string
	ActiveFile   string
	ReviewedFile string
}

// Config aggregates user-tunable settings plus resolved Paths.
type Config struct {
	Paths           Paths
	RefreshInterval time.Duration
	Columns         []string
}

type fileConfig struct {
	RefreshInterval string   `koanf:"refresh_interval"`
	Columns         []string `koanf:"columns"`
}

// Load resolves the data directory and (optionally) reads a YAML config.
// cliDataDir wins if non-empty; otherwise PROWL_DATA_DIR, then XDG_DATA_HOME/prowl,
// then ~/.local/share/prowl.
func Load(cliDataDir string) (*Config, error) {
	paths, err := resolvePaths(cliDataDir)
	if err != nil {
		return nil, err
	}
	cfg := &Config{
		Paths:           paths,
		RefreshInterval: defaultRefreshInterval,
		Columns:         defaultColumns(),
	}
	if err := mergeFileConfig(cfg, configFilePath()); err != nil {
		return nil, err
	}
	return cfg, nil
}

func resolvePaths(cliDataDir string) (Paths, error) {
	dir := cliDataDir
	if dir == "" {
		dir = os.Getenv("PROWL_DATA_DIR")
	}
	if dir == "" {
		if xdg := os.Getenv("XDG_DATA_HOME"); xdg != "" {
			dir = filepath.Join(xdg, "prowl")
		}
	}
	if dir == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return Paths{}, fmt.Errorf("resolve home dir: %w", err)
		}
		dir = filepath.Join(home, ".local", "share", "prowl")
	}
	p := Paths{DataDir: dir}
	p.ActiveFile = override("PROWL_ACTIVE", filepath.Join(dir, "prs-active.txt"))
	p.ReviewedFile = override("PROWL_REVIEWED", filepath.Join(dir, "prs-reviewed.txt"))
	return p, nil
}

func override(env, fallback string) string {
	if v := os.Getenv(env); v != "" {
		return v
	}
	return fallback
}

func configFilePath() string {
	if v := os.Getenv("PROWL_CONFIG"); v != "" {
		return v
	}
	if xdg := os.Getenv("XDG_CONFIG_HOME"); xdg != "" {
		return filepath.Join(xdg, "prowl", "config.yml")
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	return filepath.Join(home, ".config", "prowl", "config.yml")
}

func mergeFileConfig(cfg *Config, path string) error {
	if path == "" {
		return nil
	}
	if _, err := os.Stat(path); err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return fmt.Errorf("stat config: %w", err)
	}
	k := koanf.New(".")
	if err := k.Load(file.Provider(path), yaml.Parser()); err != nil {
		return fmt.Errorf("read %s: %w", path, err)
	}
	var fc fileConfig
	if err := k.Unmarshal("", &fc); err != nil {
		return fmt.Errorf("parse %s: %w", path, err)
	}
	if fc.RefreshInterval != "" {
		d, err := time.ParseDuration(fc.RefreshInterval)
		if err != nil {
			return fmt.Errorf("config refresh_interval %q: %w", fc.RefreshInterval, err)
		}
		cfg.RefreshInterval = d
	}
	if len(fc.Columns) > 0 {
		cfg.Columns = fc.Columns
	}
	return nil
}

func defaultColumns() []string {
	return []string{"PR", "Assignee", "Status", "Queue", "Pos", "ETA", "URL"}
}
