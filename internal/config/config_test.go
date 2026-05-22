package config

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestLoadCLIWins(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("PROWL_DATA_DIR", filepath.Join(dir, "env"))
	t.Setenv("XDG_DATA_HOME", filepath.Join(dir, "xdg"))
	t.Setenv("PROWL_ACTIVE", "")
	t.Setenv("PROWL_REVIEWED", "")
	t.Setenv("PROWL_CONFIG", filepath.Join(dir, "nope.yml"))

	cli := filepath.Join(dir, "cli")
	cfg, err := Load(cli)
	require.NoError(t, err)
	assert.Equal(t, cli, cfg.Paths.DataDir)
	assert.Equal(t, filepath.Join(cli, "prs-active.txt"), cfg.Paths.ActiveFile)
	assert.Equal(t, filepath.Join(cli, "prs-reviewed.txt"), cfg.Paths.ReviewedFile)
	assert.Equal(t, defaultRefreshInterval, cfg.RefreshInterval)
}

func TestLoadEnvFallback(t *testing.T) {
	dir := t.TempDir()
	env := filepath.Join(dir, "env")
	t.Setenv("PROWL_DATA_DIR", env)
	t.Setenv("PROWL_ACTIVE", "")
	t.Setenv("PROWL_REVIEWED", "")
	t.Setenv("PROWL_CONFIG", filepath.Join(dir, "nope.yml"))

	cfg, err := Load("")
	require.NoError(t, err)
	assert.Equal(t, env, cfg.Paths.DataDir)
}

func TestLoadXDGFallback(t *testing.T) {
	dir := t.TempDir()
	xdg := filepath.Join(dir, "xdg")
	t.Setenv("PROWL_DATA_DIR", "")
	t.Setenv("XDG_DATA_HOME", xdg)
	t.Setenv("PROWL_ACTIVE", "")
	t.Setenv("PROWL_REVIEWED", "")
	t.Setenv("PROWL_CONFIG", filepath.Join(dir, "nope.yml"))

	cfg, err := Load("")
	require.NoError(t, err)
	assert.Equal(t, filepath.Join(xdg, "prowl"), cfg.Paths.DataDir)
}

func TestLoadFileOverrides(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("PROWL_DATA_DIR", dir)
	t.Setenv("PROWL_ACTIVE", "")
	t.Setenv("PROWL_REVIEWED", "")

	cfgPath := filepath.Join(dir, "config.yml")
	require.NoError(t, os.WriteFile(cfgPath, []byte("refresh_interval: 1m\ncolumns:\n  - PR\n  - URL\n"), 0o644))
	t.Setenv("PROWL_CONFIG", cfgPath)

	cfg, err := Load("")
	require.NoError(t, err)
	assert.Equal(t, time.Minute, cfg.RefreshInterval)
	assert.Equal(t, []string{"PR", "URL"}, cfg.Columns)
}

func TestLoadFileInvalidDuration(t *testing.T) {
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "config.yml")
	require.NoError(t, os.WriteFile(cfgPath, []byte("refresh_interval: garbage\n"), 0o644))
	t.Setenv("PROWL_DATA_DIR", dir)
	t.Setenv("PROWL_CONFIG", cfgPath)

	_, err := Load("")
	require.Error(t, err)
}

func TestActiveReviewedEnvOverrides(t *testing.T) {
	dir := t.TempDir()
	active := filepath.Join(dir, "a.txt")
	reviewed := filepath.Join(dir, "r.txt")
	t.Setenv("PROWL_DATA_DIR", dir)
	t.Setenv("PROWL_ACTIVE", active)
	t.Setenv("PROWL_REVIEWED", reviewed)
	t.Setenv("PROWL_CONFIG", filepath.Join(dir, "nope.yml"))

	cfg, err := Load("")
	require.NoError(t, err)
	assert.Equal(t, active, cfg.Paths.ActiveFile)
	assert.Equal(t, reviewed, cfg.Paths.ReviewedFile)
}
