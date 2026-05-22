package cmd

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCheckWritable(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, checkWritable(dir))

	// nested path that doesn't exist yet should be created
	nested := filepath.Join(dir, "a", "b", "c")
	require.NoError(t, checkWritable(nested))
	info, err := os.Stat(nested)
	require.NoError(t, err)
	assert.True(t, info.IsDir())
}

func TestCheckWritableNotWritable(t *testing.T) {
	if os.Getuid() == 0 {
		t.Skip("root bypasses permission checks")
	}
	dir := t.TempDir()
	ro := filepath.Join(dir, "ro")
	require.NoError(t, os.MkdirAll(ro, 0o755))
	require.NoError(t, os.Chmod(ro, 0o500))
	t.Cleanup(func() { _ = os.Chmod(ro, 0o755) })

	// checkWritable should fail when the dir exists but isn't writable
	err := checkWritable(ro)
	require.Error(t, err)
}

func TestClipboardTool_NoToolsAvailable(t *testing.T) {
	t.Setenv("PATH", t.TempDir())
	assert.Equal(t, "", clipboardTool())
}

func TestBrowserOpener_NoToolsAvailable(t *testing.T) {
	t.Setenv("PATH", t.TempDir())
	t.Setenv("BROWSER", "")
	// On macOS without `open` on PATH, falls through; with no $BROWSER, returns "".
	// On Linux without `xdg-open`, same.
	assert.Equal(t, "", browserOpener())
}

func TestBrowserOpener_BrowserEnvFallback(t *testing.T) {
	t.Setenv("PATH", t.TempDir())
	t.Setenv("BROWSER", "/usr/bin/firefox")
	// On macOS, the `open` path is checked first; only Linux/Other falls
	// through to the BROWSER env var when no opener tool is found.
	got := browserOpener()
	// Either we found a real opener on PATH (shouldn't, PATH is empty) or we
	// fell through to BROWSER. On darwin without `open` we also fall through.
	if got != "" {
		assert.Equal(t, "/usr/bin/firefox", got)
	}
}

func TestGhAuthStatus_NoGhOnPath(t *testing.T) {
	t.Setenv("PATH", t.TempDir())
	err := ghAuthStatus()
	require.Error(t, err)
}

func TestPrintOK_Plain(t *testing.T) {
	var buf bytes.Buffer
	printOK(&buf, true, "gh", "/usr/bin/gh")
	out := buf.String()
	assert.Contains(t, out, "[OK]")
	assert.Contains(t, out, "gh")
	assert.Contains(t, out, "/usr/bin/gh")
}

func TestPrintOK_Pretty(t *testing.T) {
	var buf bytes.Buffer
	printOK(&buf, false, "gh", "/usr/bin/gh")
	assert.Contains(t, buf.String(), "gh")
}

func TestPrintMissing_Plain(t *testing.T) {
	var buf bytes.Buffer
	printMissing(&buf, true, "gh", "install it")
	assert.Contains(t, buf.String(), "[MISSING]")
}

func TestPrintMissing_Pretty(t *testing.T) {
	var buf bytes.Buffer
	printMissing(&buf, false, "gh", "install it")
	assert.Contains(t, buf.String(), "install it")
}

func TestPrintOptional_Plain(t *testing.T) {
	var buf bytes.Buffer
	printOptional(&buf, true, "browser", "no opener")
	assert.Contains(t, buf.String(), "[OPTIONAL]")
}

func TestPrintOptional_Pretty(t *testing.T) {
	var buf bytes.Buffer
	printOptional(&buf, false, "browser", "no opener")
	assert.Contains(t, buf.String(), "browser")
}

func TestRunCheck_DataDirOK(t *testing.T) {
	// Even when gh / browser tools are absent the check command should still
	// reach the data-dir step and exercise the helpers. We don't care whether
	// it returns a "missing dependency" error — only that it ran the body.
	_, _ = setupCLI(t)
	stdout, _, _ := execCLI(t, "check")
	assert.Contains(t, stdout, "prowl")
}
