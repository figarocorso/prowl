package data

import (
	"context"
	"errors"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParseURL(t *testing.T) {
	owner, repo, num, err := ParseURL("https://github.com/acme/api/pull/1234")
	require.NoError(t, err)
	assert.Equal(t, "acme", owner)
	assert.Equal(t, "api", repo)
	assert.Equal(t, 1234, num)

	owner, repo, num, err = ParseURL("https://github.com/acme/api/pull/1234/files")
	require.NoError(t, err)
	assert.Equal(t, 1234, num)
	_ = owner
	_ = repo

	_, _, _, err = ParseURL("https://example.com/foo/bar")
	require.Error(t, err)
}

func TestCanonicalURL(t *testing.T) {
	c, err := CanonicalURL("https://github.com/acme/api/pull/12/files#diff-abc")
	require.NoError(t, err)
	assert.Equal(t, "https://github.com/acme/api/pull/12", c)
}

func loadFixtures(t *testing.T) *MockClient {
	t.Helper()
	m := NewMockClient()
	require.NoError(t, m.LoadFixtures(filepath.Join("testdata", "fixtures.json")))
	return m
}

func TestMockClientFetch(t *testing.T) {
	m := loadFixtures(t)
	pr, err := m.Fetch(context.Background(), "https://github.com/acme/api/pull/1234")
	require.NoError(t, err)
	assert.Equal(t, 1234, pr.Number)
	assert.Equal(t, "OPEN", pr.State)
	require.NotNil(t, pr.Queue)
	assert.Equal(t, 2, pr.Queue.Position)
	assert.Equal(t, 7*time.Minute, pr.Queue.ETA)
}

func TestMockClientFetchBatchPreservesOrder(t *testing.T) {
	m := loadFixtures(t)
	urls := []string{
		"https://github.com/acme/api/pull/1235",
		"https://github.com/acme/api/pull/1234",
		"https://github.com/acme/api/pull/1198",
	}
	results := m.FetchBatch(context.Background(), urls)
	require.Len(t, results, 3)
	for i, r := range results {
		assert.Equal(t, urls[i], r.URL, "url at index %d", i)
		assert.NoError(t, r.Err)
	}
	assert.Equal(t, 1235, results[0].PR.Number)
	assert.Equal(t, 1234, results[1].PR.Number)
	assert.Equal(t, 1198, results[2].PR.Number)
}

func TestMockClientFetchBatchPerURLError(t *testing.T) {
	m := loadFixtures(t)
	boom := errors.New("boom")
	m.Errors["https://github.com/acme/api/pull/1234"] = boom

	results := m.FetchBatch(context.Background(), []string{
		"https://github.com/acme/api/pull/1234",
		"https://github.com/acme/api/pull/1198",
	})
	require.Len(t, results, 2)
	assert.ErrorIs(t, results[0].Err, boom)
	assert.NoError(t, results[1].Err)
	assert.Equal(t, 1198, results[1].PR.Number)
}

func TestStatusLabel(t *testing.T) {
	cases := map[string]PR{
		"open":         {State: "OPEN", MergeStateStatus: "CLEAN"},
		"open/blocked": {State: "OPEN", MergeStateStatus: "BLOCKED"},
		"draft":        {State: "OPEN", IsDraft: true},
		"merged":       {State: "MERGED"},
		"closed":       {State: "CLOSED"},
		"unknown":      {State: ""},
	}
	for want, pr := range cases {
		assert.Equal(t, want, StatusLabel(pr), "case %s", want)
	}
}

func TestQueueLabel(t *testing.T) {
	assert.Equal(t, "-", QueueLabel(PR{}))
	assert.Equal(t, "queued (mergeable)", QueueLabel(PR{Queue: &MergeQueueEntry{State: "MERGEABLE"}}))
	assert.Equal(t, "queued (awaiting checks)", QueueLabel(PR{Queue: &MergeQueueEntry{State: "AWAITING_CHECKS"}}))
	assert.Equal(t, "queued (locked)", QueueLabel(PR{Queue: &MergeQueueEntry{State: "LOCKED"}}))
	assert.Equal(t, "queued (something else)", QueueLabel(PR{Queue: &MergeQueueEntry{State: "SOMETHING_ELSE"}}))
}

func TestQueuePositionLabel(t *testing.T) {
	assert.Equal(t, "-", QueuePositionLabel(PR{}))
	assert.Equal(t, "-", QueuePositionLabel(PR{Queue: &MergeQueueEntry{Position: 0}}))
	assert.Equal(t, "-", QueuePositionLabel(PR{Queue: &MergeQueueEntry{Position: -1}}))
	assert.Equal(t, "3", QueuePositionLabel(PR{Queue: &MergeQueueEntry{Position: 3}}))
}

func TestETALabel(t *testing.T) {
	assert.Equal(t, "-", ETALabel(PR{}))
	assert.Equal(t, "~30s", ETALabel(PR{Queue: &MergeQueueEntry{ETA: 30 * time.Second}}))
	assert.Equal(t, "~7m", ETALabel(PR{Queue: &MergeQueueEntry{ETA: 7 * time.Minute}}))
	assert.Equal(t, "~2h", ETALabel(PR{Queue: &MergeQueueEntry{ETA: 2 * time.Hour}}))
}

func TestAssigneesLabel(t *testing.T) {
	assert.Equal(t, "-", AssigneesLabel(PR{}))
	assert.Equal(t, "alice,bob", AssigneesLabel(PR{Assignees: []string{"alice", "bob"}}))
}

func TestIsTerminal(t *testing.T) {
	assert.True(t, IsTerminal(PR{State: "MERGED"}))
	assert.True(t, IsTerminal(PR{State: "CLOSED"}))
	assert.False(t, IsTerminal(PR{State: "OPEN"}))
}

func TestQueueLabelShort(t *testing.T) {
	assert.Equal(t, "-", QueueLabelShort(PR{}))
	assert.Equal(t, "ready", QueueLabelShort(PR{Queue: &MergeQueueEntry{State: "MERGEABLE"}}))
	assert.Equal(t, "checks", QueueLabelShort(PR{Queue: &MergeQueueEntry{State: "AWAITING_CHECKS"}}))
	assert.Equal(t, "locked", QueueLabelShort(PR{Queue: &MergeQueueEntry{State: "LOCKED"}}))
	assert.Equal(t, "blocked", QueueLabelShort(PR{Queue: &MergeQueueEntry{State: "UNMERGEABLE"}}))
	assert.Equal(t, "queued", QueueLabelShort(PR{Queue: &MergeQueueEntry{State: "QUEUED"}}))
	assert.Equal(t, "weird", QueueLabelShort(PR{Queue: &MergeQueueEntry{State: "WEIRD"}}))
}

func TestShortURL(t *testing.T) {
	assert.Equal(t, "acme/api/pull/42", ShortURL("https://github.com/acme/api/pull/42"))
	assert.Equal(t, "acme/api/pull/42", ShortURL("https://www.github.com/acme/api/pull/42"))
	assert.Equal(t, "acme/api/pull/42", ShortURL("http://github.com/acme/api/pull/42"))
	assert.Equal(t, "https://gitlab.com/x/y", ShortURL("https://gitlab.com/x/y"))
}

func TestComputeStats(t *testing.T) {
	results := []Result{
		{PR: PR{State: "OPEN"}},
		{PR: PR{State: "OPEN", IsDraft: true}},
		{PR: PR{State: "OPEN", MergeStateStatus: "BLOCKED"}},
		{PR: PR{State: "MERGED"}},
		{PR: PR{State: "CLOSED"}},
		{PR: PR{State: "OPEN", Queue: &MergeQueueEntry{State: "MERGEABLE"}}},
		{Err: assert.AnError},
	}
	s := ComputeStats(results, 4)
	assert.Equal(t, 7, s.Active)
	assert.Equal(t, 4, s.Reviewed)
	assert.Equal(t, 11, s.Total)
	assert.Equal(t, 2, s.Open)
	assert.Equal(t, 1, s.Draft)
	assert.Equal(t, 1, s.Blocked)
	assert.Equal(t, 1, s.Merged)
	assert.Equal(t, 1, s.Closed)
	assert.Equal(t, 1, s.Queued)
	assert.Equal(t, 1, s.Errors)
}
