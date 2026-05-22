package store

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func newStore(t *testing.T) *Store {
	t.Helper()
	dir := t.TempDir()
	s, err := New(filepath.Join(dir, "prs-active.txt"), filepath.Join(dir, "prs-reviewed.txt"))
	require.NoError(t, err)
	return s
}

func TestNewCreatesFiles(t *testing.T) {
	s := newStore(t)
	_, err := os.Stat(s.ActivePath)
	require.NoError(t, err)
	_, err = os.Stat(s.ReviewedPath)
	require.NoError(t, err)
}

func TestAddDeduplicates(t *testing.T) {
	s := newStore(t)
	added, err := s.Add("https://github.com/acme/api/pull/1")
	require.NoError(t, err)
	assert.True(t, added)

	added, err = s.Add("https://github.com/acme/api/pull/1")
	require.NoError(t, err)
	assert.False(t, added)

	urls, err := s.Active()
	require.NoError(t, err)
	assert.Equal(t, []string{"https://github.com/acme/api/pull/1"}, urls)
}

func TestAddDeduplicatesAgainstReviewed(t *testing.T) {
	s := newStore(t)
	require.NoError(t, os.WriteFile(s.ReviewedPath, []byte("https://github.com/acme/api/pull/9\n"), 0o644))
	added, err := s.Add("https://github.com/acme/api/pull/9")
	require.NoError(t, err)
	assert.False(t, added)
}

func TestMoveActiveToReviewedPreservesOthers(t *testing.T) {
	s := newStore(t)
	for _, u := range []string{
		"https://github.com/acme/api/pull/1",
		"https://github.com/acme/api/pull/2",
		"https://github.com/acme/api/pull/3",
	} {
		_, err := s.Add(u)
		require.NoError(t, err)
	}

	moved, err := s.MoveActiveToReviewed([]string{"https://github.com/acme/api/pull/2"})
	require.NoError(t, err)
	assert.Equal(t, 1, moved)

	active, err := s.Active()
	require.NoError(t, err)
	assert.Equal(t, []string{
		"https://github.com/acme/api/pull/1",
		"https://github.com/acme/api/pull/3",
	}, active)

	reviewed, err := s.Reviewed()
	require.NoError(t, err)
	assert.Equal(t, []string{"https://github.com/acme/api/pull/2"}, reviewed)
}

func TestRemoveDeletesFromBothLists(t *testing.T) {
	s := newStore(t)
	require.NoError(t, os.WriteFile(s.ActivePath, []byte("https://github.com/acme/api/pull/1\n"), 0o644))
	require.NoError(t, os.WriteFile(s.ReviewedPath, []byte("https://github.com/acme/api/pull/1\n"), 0o644))

	removed, err := s.Remove("https://github.com/acme/api/pull/1")
	require.NoError(t, err)
	assert.Equal(t, 2, removed)

	active, err := s.Active()
	require.NoError(t, err)
	assert.Empty(t, active)
	reviewed, err := s.Reviewed()
	require.NoError(t, err)
	assert.Empty(t, reviewed)
}

func TestReadURLsIgnoresCommentsAndBlanks(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "list.txt")
	body := "# header\n" +
		"\n" +
		"https://github.com/acme/api/pull/1\n" +
		"  # indented\n" +
		"\thttps://github.com/acme/api/pull/2\n"
	require.NoError(t, os.WriteFile(path, []byte(body), 0o644))

	urls, err := ReadURLs(path)
	require.NoError(t, err)
	assert.Equal(t, []string{
		"https://github.com/acme/api/pull/1",
		"https://github.com/acme/api/pull/2",
	}, urls)
}

func TestReadURLsStripsLegacyPrefix(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "list.txt")
	require.NoError(t, os.WriteFile(path, []byte("MERGED|https://github.com/acme/api/pull/1\n"), 0o644))

	urls, err := ReadURLs(path)
	require.NoError(t, err)
	assert.Equal(t, []string{"https://github.com/acme/api/pull/1"}, urls)
}

func TestMigrateLegacyFiles(t *testing.T) {
	dir := t.TempDir()
	active := filepath.Join(dir, "prs-active.txt")
	reviewed := filepath.Join(dir, "prs-reviewed.txt")
	require.NoError(t, os.WriteFile(filepath.Join(dir, "prs-unmerged.txt"), []byte("https://github.com/acme/api/pull/10\n"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "prs-merged.txt"), []byte("https://github.com/acme/api/pull/20\n"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "prs-closed.txt"), []byte("https://github.com/acme/api/pull/30\n"), 0o644))

	_, err := New(active, reviewed)
	require.NoError(t, err)

	urls, err := ReadURLs(active)
	require.NoError(t, err)
	assert.Equal(t, []string{"https://github.com/acme/api/pull/10"}, urls)

	urls, err = ReadURLs(reviewed)
	require.NoError(t, err)
	assert.ElementsMatch(t, []string{
		"https://github.com/acme/api/pull/20",
		"https://github.com/acme/api/pull/30",
	}, urls)

	_, err = os.Stat(filepath.Join(dir, "prs-unmerged.txt"))
	assert.True(t, os.IsNotExist(err), "legacy unmerged should be moved")
	_, err = os.Stat(filepath.Join(dir, "prs-merged.txt"))
	assert.True(t, os.IsNotExist(err))
	_, err = os.Stat(filepath.Join(dir, "prs-closed.txt"))
	assert.True(t, os.IsNotExist(err))
}
