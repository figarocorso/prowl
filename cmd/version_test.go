package cmd

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSetVersionInfo(t *testing.T) {
	origV, origC, origD := buildVersion, buildCommit, buildDate
	origRootVer := rootCmd.Version
	t.Cleanup(func() {
		buildVersion = origV
		buildCommit = origC
		buildDate = origD
		rootCmd.Version = origRootVer
	})

	SetVersionInfo("v1.2.3", "deadbeef", "2026-05-22T10:00:00Z")
	assert.Equal(t, "v1.2.3", buildVersion)
	assert.Equal(t, "deadbeef", buildCommit)
	assert.Equal(t, "2026-05-22T10:00:00Z", buildDate)
	assert.Contains(t, rootCmd.Version, "v1.2.3")
	assert.Contains(t, rootCmd.Version, "deadbeef")
}

func TestVersionCommandOutput(t *testing.T) {
	origV, origC, origD := buildVersion, buildCommit, buildDate
	t.Cleanup(func() {
		buildVersion = origV
		buildCommit = origC
		buildDate = origD
	})
	buildVersion = "v9.9.9"
	buildCommit = "cafef00d"
	buildDate = "2026-01-01"

	stdout, _, err := execCLI(t, "version")
	require.NoError(t, err)
	assert.Contains(t, stdout, "v9.9.9")
	assert.Contains(t, stdout, "cafef00d")
	assert.Contains(t, stdout, "2026-01-01")
}
