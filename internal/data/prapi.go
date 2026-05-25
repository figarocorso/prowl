// Package data fetches Pull Request metadata from GitHub.
//
// The PRClient interface keeps the GitHub-specific code behind a small surface
// so the TUI/CLI can be exercised against a mock in tests.
package data

import (
	"context"
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/cli/go-gh/v2/pkg/api"
)

// PR is the structured view of a single GitHub Pull Request.
type PR struct {
	URL              string           `json:"url"`
	Owner            string           `json:"owner"`
	Repo             string           `json:"repo"`
	Number           int              `json:"number"`
	Title            string           `json:"title,omitempty"`
	State            string           `json:"state"`
	MergeStateStatus string           `json:"merge_state_status,omitempty"`
	ReviewDecision   string           `json:"review_decision,omitempty"`
	CheckRollupState string           `json:"check_rollup_state,omitempty"`
	IsDraft          bool             `json:"is_draft"`
	Assignees        []string         `json:"assignees,omitempty"`
	Queue            *MergeQueueEntry `json:"queue,omitempty"`
}

// MergeQueueEntry models a PR's membership in GitHub's native merge queue.
type MergeQueueEntry struct {
	State    string        `json:"state"`
	Position int           `json:"position"`
	ETA      time.Duration `json:"eta,omitempty"`
}

// Result is a per-URL outcome from FetchBatch.
type Result struct {
	URL string
	PR  PR
	Err error
}

// PRClient is the surface used by the CLI/TUI/MCP layers to read PR state.
type PRClient interface {
	Fetch(ctx context.Context, url string) (PR, error)
	FetchBatch(ctx context.Context, urls []string) []Result
}

var urlRE = regexp.MustCompile(`^https?://github\.com/([^/]+)/([^/]+)/pull/(\d+)`)

// ParseURL extracts (owner, repo, number) from a canonical GitHub PR URL.
// Returns an error if the URL doesn't match.
func ParseURL(url string) (owner, repo string, number int, err error) {
	m := urlRE.FindStringSubmatch(url)
	if m == nil {
		return "", "", 0, fmt.Errorf("not a GitHub PR URL: %s", url)
	}
	n, err := strconv.Atoi(m[3])
	if err != nil {
		return "", "", 0, fmt.Errorf("bad PR number: %w", err)
	}
	return m[1], m[2], n, nil
}

// CanonicalURL returns just the canonical PR URL (no fragments/queries).
func CanonicalURL(raw string) (string, error) {
	owner, repo, n, err := ParseURL(raw)
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("https://github.com/%s/%s/pull/%d", owner, repo, n), nil
}

const defaultConcurrency = 8

// GHClient implements PRClient using go-gh's GraphQL client.
type GHClient struct {
	gql         graphqlClient
	concurrency int
}

type graphqlClient interface {
	Do(query string, variables map[string]any, response any) error
}

// NewGHClient builds a real GitHub client. It honours the same auth as `gh`.
func NewGHClient() (*GHClient, error) {
	client, err := api.DefaultGraphQLClient()
	if err != nil {
		return nil, fmt.Errorf("graphql client: %w", err)
	}
	return &GHClient{gql: client, concurrency: defaultConcurrency}, nil
}

const prQuery = `query ($owner:String!,$repo:String!,$num:Int!) {
  repository(owner:$owner,name:$repo) {
    pullRequest(number:$num) {
      number
      title
      state
      mergeStateStatus
      reviewDecision
      isDraft
      assignees(first:10) { nodes { login } }
      mergeQueueEntry { state position estimatedTimeToMerge }
    }
  }
}`

type prGQLResponse struct {
	Repository struct {
		PullRequest *struct {
			Number           int    `json:"number"`
			Title            string `json:"title"`
			State            string `json:"state"`
			MergeStateStatus string `json:"mergeStateStatus"`
			ReviewDecision   string `json:"reviewDecision"`
			IsDraft          bool   `json:"isDraft"`
			Assignees        struct {
				Nodes []struct {
					Login string `json:"login"`
				} `json:"nodes"`
			} `json:"assignees"`
			MergeQueueEntry *struct {
				State                string `json:"state"`
				Position             int    `json:"position"`
				EstimatedTimeToMerge *int   `json:"estimatedTimeToMerge"`
			} `json:"mergeQueueEntry"`
		} `json:"pullRequest"`
	} `json:"repository"`
}

const checkRollupQuery = `query ($owner:String!,$repo:String!,$num:Int!) {
  repository(owner:$owner,name:$repo) {
    pullRequest(number:$num) {
      commits(last:1) { nodes { commit { statusCheckRollup { state } } } }
    }
  }
}`

type checkRollupGQLResponse struct {
	Repository struct {
		PullRequest *struct {
			Commits struct {
				Nodes []struct {
					Commit struct {
						StatusCheckRollup *struct {
							State string `json:"state"`
						} `json:"statusCheckRollup"`
					} `json:"commit"`
				} `json:"nodes"`
			} `json:"commits"`
		} `json:"pullRequest"`
	} `json:"repository"`
}

// needsCheckRollup reports whether a PR's "blocked" reason is ambiguous from
// the main query (BLOCKED + approved/no-decision, not draft, not queued) and
// therefore benefits from the secondary statusCheckRollup query.
func needsCheckRollup(pr PR) bool {
	if pr.Queue != nil || pr.IsDraft {
		return false
	}
	if !strings.EqualFold(pr.State, "OPEN") {
		return false
	}
	if !strings.EqualFold(pr.MergeStateStatus, "BLOCKED") {
		return false
	}
	switch strings.ToUpper(pr.ReviewDecision) {
	case "APPROVED", "":
		return true
	}
	return false
}

// fetchCheckRollup runs the secondary GraphQL query that returns the aggregate
// statusCheckRollup state for the PR's latest commit. Returns "" when the PR
// has no commit/rollup yet, so callers can treat it as "unknown".
func (c *GHClient) fetchCheckRollup(ctx context.Context, owner, repo string, num int) (string, error) {
	_ = ctx
	var resp checkRollupGQLResponse
	if err := c.gql.Do(checkRollupQuery, map[string]any{
		"owner": owner,
		"repo":  repo,
		"num":   num,
	}, &resp); err != nil {
		return "", err
	}
	if resp.Repository.PullRequest == nil || len(resp.Repository.PullRequest.Commits.Nodes) == 0 {
		return "", nil
	}
	rollup := resp.Repository.PullRequest.Commits.Nodes[0].Commit.StatusCheckRollup
	if rollup == nil {
		return "", nil
	}
	return rollup.State, nil
}

// Fetch retrieves a single PR by URL.
func (c *GHClient) Fetch(ctx context.Context, url string) (PR, error) {
	owner, repo, num, err := ParseURL(url)
	if err != nil {
		return PR{}, err
	}
	canonical := fmt.Sprintf("https://github.com/%s/%s/pull/%d", owner, repo, num)

	var resp prGQLResponse
	if err := c.gql.Do(prQuery, map[string]any{
		"owner": owner,
		"repo":  repo,
		"num":   num,
	}, &resp); err != nil {
		return PR{}, fmt.Errorf("graphql %s: %w", canonical, err)
	}
	if resp.Repository.PullRequest == nil {
		return PR{}, fmt.Errorf("pull request not found: %s", canonical)
	}
	p := resp.Repository.PullRequest
	out := PR{
		URL:              canonical,
		Owner:            owner,
		Repo:             repo,
		Number:           p.Number,
		Title:            p.Title,
		State:            p.State,
		MergeStateStatus: p.MergeStateStatus,
		ReviewDecision:   p.ReviewDecision,
		IsDraft:          p.IsDraft,
	}
	for _, a := range p.Assignees.Nodes {
		out.Assignees = append(out.Assignees, a.Login)
	}
	if p.MergeQueueEntry != nil {
		entry := &MergeQueueEntry{
			State:    p.MergeQueueEntry.State,
			Position: p.MergeQueueEntry.Position,
		}
		if p.MergeQueueEntry.EstimatedTimeToMerge != nil {
			entry.ETA = time.Duration(*p.MergeQueueEntry.EstimatedTimeToMerge) * time.Second
		}
		out.Queue = entry
	}
	if needsCheckRollup(out) {
		// Best-effort: a rollup fetch error shouldn't blank out the rest of
		// the PR. DetailsLabel falls back to "blocked" when state is empty.
		if state, err := c.fetchCheckRollup(ctx, owner, repo, num); err == nil {
			out.CheckRollupState = state
		}
	}
	return out, nil
}

// FetchBatch concurrently fetches many PRs. Returned slice is in input order.
// Per-URL errors do not abort the batch — they're attached to the corresponding
// Result instead.
func (c *GHClient) FetchBatch(ctx context.Context, urls []string) []Result {
	return fetchBatch(ctx, c.Fetch, c.concurrency, urls)
}

func fetchBatch(ctx context.Context, fetch func(context.Context, string) (PR, error), concurrency int, urls []string) []Result {
	if concurrency <= 0 {
		concurrency = defaultConcurrency
	}
	results := make([]Result, len(urls))
	sem := make(chan struct{}, concurrency)
	var wg sync.WaitGroup
	for i, url := range urls {
		i, url := i, url
		wg.Add(1)
		sem <- struct{}{}
		go func() {
			defer wg.Done()
			defer func() { <-sem }()
			if ctx.Err() != nil {
				results[i] = Result{URL: url, Err: ctx.Err()}
				return
			}
			pr, err := fetch(ctx, url)
			results[i] = Result{URL: url, PR: pr, Err: err}
		}()
	}
	wg.Wait()
	return results
}
