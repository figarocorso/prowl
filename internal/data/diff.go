package data

import (
	"bytes"
	"context"
	"fmt"
	"os/exec"
	"strings"
)

// DiffSummary is a light-weight digest of a unified diff.
type DiffSummary struct {
	Files     int `json:"files"`
	Additions int `json:"additions"`
	Deletions int `json:"deletions"`
}

// FetchDiff returns the raw unified diff text for a PR by shelling out to
// `gh api` with the diff Accept header. This avoids re-implementing GitHub
// auth — anywhere `gh` is logged in, this works.
func FetchDiff(ctx context.Context, url string) (string, error) {
	owner, repo, num, err := ParseURL(url)
	if err != nil {
		return "", err
	}
	cmd := exec.CommandContext(ctx, "gh", "api",
		fmt.Sprintf("repos/%s/%s/pulls/%d", owner, repo, num),
		"--header", "Accept: application/vnd.github.v3.diff",
	)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("gh api: %w (%s)", err, strings.TrimSpace(stderr.String()))
	}
	return stdout.String(), nil
}

// SummarizeDiff counts files, additions, and deletions in a unified diff.
func SummarizeDiff(diff string) DiffSummary {
	var s DiffSummary
	for _, line := range strings.Split(diff, "\n") {
		switch {
		case strings.HasPrefix(line, "diff --git "):
			s.Files++
		case strings.HasPrefix(line, "+++"), strings.HasPrefix(line, "---"):
			// header lines; ignore
		case strings.HasPrefix(line, "+"):
			s.Additions++
		case strings.HasPrefix(line, "-"):
			s.Deletions++
		}
	}
	return s
}
