package data

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
)

// MockClient is an in-memory PRClient. Tests populate PRs (and optionally
// Errors) keyed by URL.
type MockClient struct {
	PRs    map[string]PR
	Errors map[string]error
}

// NewMockClient builds an empty MockClient.
func NewMockClient() *MockClient {
	return &MockClient{
		PRs:    map[string]PR{},
		Errors: map[string]error{},
	}
}

// LoadFixtures reads a JSON file containing a map of canonical URL → PR and
// merges it into the mock. Multiple calls accumulate.
func (m *MockClient) LoadFixtures(path string) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("read fixture %s: %w", path, err)
	}
	var raw map[string]PR
	if err := json.Unmarshal(data, &raw); err != nil {
		return fmt.Errorf("parse fixture %s: %w", path, err)
	}
	for url, pr := range raw {
		if pr.URL == "" {
			pr.URL = url
		}
		m.PRs[url] = pr
	}
	return nil
}

// Fetch looks up a single URL.
func (m *MockClient) Fetch(_ context.Context, url string) (PR, error) {
	if err, ok := m.Errors[url]; ok {
		return PR{}, err
	}
	pr, ok := m.PRs[url]
	if !ok {
		return PR{}, fmt.Errorf("mock: PR not found: %s", url)
	}
	return pr, nil
}

// FetchBatch fans out across the mock concurrently to mirror the real client.
func (m *MockClient) FetchBatch(ctx context.Context, urls []string) []Result {
	return fetchBatch(ctx, m.Fetch, defaultConcurrency, urls)
}
