package cmd

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/figarocorso/prowl/internal/data"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func setupCLI(t *testing.T) (*data.MockClient, string) {
	t.Helper()
	dir := t.TempDir()
	t.Setenv("PROWL_DATA_DIR", dir)
	t.Setenv("PROWL_PROFILE", "")
	t.Setenv("PROWL_ACTIVE", "")
	t.Setenv("PROWL_REVIEWED", "")
	t.Setenv("PROWL_CONFIG", filepath.Join(dir, "nope.yml"))

	mock := data.NewMockClient()
	require.NoError(t, mock.LoadFixtures(filepath.Join("..", "internal", "data", "testdata", "fixtures.json")))

	origFactory := clientFactory
	clientFactory = func() (data.PRClient, error) { return mock, nil }
	origDataDir := dataDir
	dataDir = ""
	origProfile := profile
	profile = ""
	origJSON := jsonOut
	t.Cleanup(func() {
		clientFactory = origFactory
		dataDir = origDataDir
		profile = origProfile
		jsonOut = origJSON
	})
	return mock, dir
}

func execCLI(t *testing.T, args ...string) (string, string, error) {
	t.Helper()
	var stdout, stderr bytes.Buffer
	rootCmd.SetOut(&stdout)
	rootCmd.SetErr(&stderr)
	rootCmd.SetArgs(args)
	err := rootCmd.Execute()
	return stdout.String(), stderr.String(), err
}

func TestAddListJSON(t *testing.T) {
	_, _ = setupCLI(t)

	_, _, err := execCLI(t, "add", "https://github.com/acme/api/pull/1234")
	require.NoError(t, err)
	_, _, err = execCLI(t, "add", "https://github.com/acme/api/pull/1235")
	require.NoError(t, err)

	stdout, _, err := execCLI(t, "list", "--json")
	require.NoError(t, err)

	var rows []jsonRow
	require.NoError(t, json.Unmarshal([]byte(stdout), &rows))
	require.Len(t, rows, 2)
	assert.Equal(t, 1234, rows[0].Number)
	assert.Equal(t, "open", rows[0].Status)
	assert.Equal(t, "open/blocked", rows[1].Status)
}

func TestListTableHasHeader(t *testing.T) {
	_, _ = setupCLI(t)
	_, _, err := execCLI(t, "add", "https://github.com/acme/api/pull/1234")
	require.NoError(t, err)

	stdout, _, err := execCLI(t, "list")
	require.NoError(t, err)
	assert.Contains(t, stdout, "PR")
	assert.Contains(t, stdout, "Status")
	assert.Contains(t, stdout, "#1234")
}

func TestArchiveMovesTerminalPRs(t *testing.T) {
	_, dir := setupCLI(t)

	_, _, err := execCLI(t, "add", "https://github.com/acme/api/pull/1198")
	require.NoError(t, err)
	_, _, err = execCLI(t, "add", "https://github.com/acme/api/pull/1234")
	require.NoError(t, err)

	stdout, _, err := execCLI(t, "archive")
	require.NoError(t, err)
	assert.Contains(t, stdout, "archived 1 PR")

	activeBytes, err := os.ReadFile(filepath.Join(dir, "prs-active.txt"))
	require.NoError(t, err)
	active := strings.TrimSpace(string(activeBytes))
	assert.Equal(t, "https://github.com/acme/api/pull/1234", active)

	reviewedBytes, err := os.ReadFile(filepath.Join(dir, "prs-reviewed.txt"))
	require.NoError(t, err)
	assert.Contains(t, string(reviewedBytes), "https://github.com/acme/api/pull/1198")
}

func TestRemoveCommand(t *testing.T) {
	_, dir := setupCLI(t)
	_, _, err := execCLI(t, "add", "https://github.com/acme/api/pull/1234")
	require.NoError(t, err)

	stdout, _, err := execCLI(t, "remove", "https://github.com/acme/api/pull/1234")
	require.NoError(t, err)
	assert.Contains(t, stdout, "removed")

	activeBytes, err := os.ReadFile(filepath.Join(dir, "prs-active.txt"))
	require.NoError(t, err)
	assert.Empty(t, strings.TrimSpace(string(activeBytes)))
}

func TestAddRejectsNonGithubURL(t *testing.T) {
	_, _ = setupCLI(t)
	stdout, _, err := execCLI(t, "add", "https://example.com/foo/bar")
	require.NoError(t, err)
	assert.Contains(t, stdout, "not a GitHub PR URL")
}

func TestGetJSON(t *testing.T) {
	_, _ = setupCLI(t)
	stdout, _, err := execCLI(t, "get", "https://github.com/acme/api/pull/1234", "--json")
	require.NoError(t, err)

	var pr data.PR
	require.NoError(t, json.Unmarshal([]byte(stdout), &pr))
	assert.Equal(t, 1234, pr.Number)
	assert.Equal(t, "OPEN", pr.State)
}

func TestListOpenFilter(t *testing.T) {
	_, _ = setupCLI(t)
	for _, u := range []string{
		"https://github.com/acme/api/pull/1234",
		"https://github.com/acme/api/pull/1198",
	} {
		_, _, err := execCLI(t, "add", u)
		require.NoError(t, err)
	}

	stdout, _, err := execCLI(t, "list", "--json", "--open")
	require.NoError(t, err)

	var rows []jsonRow
	require.NoError(t, json.Unmarshal([]byte(stdout), &rows))
	require.Len(t, rows, 1)
	assert.Equal(t, 1234, rows[0].Number)
}
