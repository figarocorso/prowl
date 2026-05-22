package mcp

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"io"
	"path/filepath"
	"strings"
	"testing"

	"github.com/figarocorso/prowl/internal/config"
	"github.com/figarocorso/prowl/internal/data"
	"github.com/figarocorso/prowl/internal/store"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func newTestServer(t *testing.T, allowMutations bool) (*Server, *data.MockClient, *store.Store, *bytes.Buffer, *io.PipeWriter) {
	t.Helper()
	dir := t.TempDir()
	s, err := store.New(filepath.Join(dir, "a.txt"), filepath.Join(dir, "r.txt"))
	require.NoError(t, err)

	mock := data.NewMockClient()
	require.NoError(t, mock.LoadFixtures(filepath.Join("..", "..", "internal", "data", "testdata", "fixtures.json")))

	pr, pw := io.Pipe()
	out := &bytes.Buffer{}
	srv := NewServer(Options{
		Config:         &config.Config{Paths: config.Paths{DataDir: dir}},
		Store:          s,
		Client:         mock,
		In:             pr,
		Out:            out,
		AllowMutations: allowMutations,
		DiffFetcher: func(_ context.Context, url string) (string, error) {
			return "diff --git a/x b/x\n+added\n+more\n-removed\n", nil
		},
		Version: "test",
	})
	return srv, mock, s, out, pw
}

func sendAndCollect(t *testing.T, srv *Server, out *bytes.Buffer, in *io.PipeWriter, messages []string) []map[string]any {
	t.Helper()
	errc := make(chan error, 1)
	go func() {
		errc <- srv.Run(context.Background())
	}()
	for _, m := range messages {
		_, err := in.Write([]byte(m + "\n"))
		require.NoError(t, err)
	}
	require.NoError(t, in.Close())
	require.NoError(t, <-errc)

	scanner := bufio.NewScanner(strings.NewReader(out.String()))
	scanner.Buffer(make([]byte, 0, 64*1024), 4*1024*1024)
	var responses []map[string]any
	for scanner.Scan() {
		var r map[string]any
		require.NoError(t, json.Unmarshal(scanner.Bytes(), &r))
		responses = append(responses, r)
	}
	return responses
}

func TestInitializeAndToolsList(t *testing.T) {
	srv, _, _, out, in := newTestServer(t, false)
	resps := sendAndCollect(t, srv, out, in, []string{
		`{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2024-11-05"}}`,
		`{"jsonrpc":"2.0","method":"notifications/initialized"}`,
		`{"jsonrpc":"2.0","id":2,"method":"tools/list"}`,
	})
	require.Len(t, resps, 2)
	assert.Equal(t, float64(1), resps[0]["id"])
	result := resps[0]["result"].(map[string]any)
	info := result["serverInfo"].(map[string]any)
	assert.Equal(t, "prowl", info["name"])

	tools := resps[1]["result"].(map[string]any)["tools"].([]any)
	names := []string{}
	for _, t := range tools {
		names = append(names, t.(map[string]any)["name"].(string))
	}
	assert.ElementsMatch(t, []string{"list_prs", "get_pr", "get_pr_diff"}, names)
}

func TestListPRsTool(t *testing.T) {
	srv, _, s, out, in := newTestServer(t, false)
	_, err := s.Add("https://github.com/acme/api/pull/1234")
	require.NoError(t, err)
	_, err = s.Add("https://github.com/acme/api/pull/1198")
	require.NoError(t, err)

	resps := sendAndCollect(t, srv, out, in, []string{
		`{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"list_prs","arguments":{"source":"active"}}}`,
	})
	require.Len(t, resps, 1)
	result := resps[0]["result"].(map[string]any)
	content := result["content"].([]any)
	text := content[0].(map[string]any)["text"].(string)

	var rows []map[string]any
	require.NoError(t, json.Unmarshal([]byte(text), &rows))
	require.Len(t, rows, 2)
	assert.Equal(t, float64(1234), rows[0]["number"])
}

func TestListPRsStatusFilter(t *testing.T) {
	srv, _, s, out, in := newTestServer(t, false)
	for _, u := range []string{
		"https://github.com/acme/api/pull/1234",
		"https://github.com/acme/api/pull/1198",
	} {
		_, err := s.Add(u)
		require.NoError(t, err)
	}

	resps := sendAndCollect(t, srv, out, in, []string{
		`{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"list_prs","arguments":{"status":"merged"}}}`,
	})
	require.Len(t, resps, 1)
	text := resps[0]["result"].(map[string]any)["content"].([]any)[0].(map[string]any)["text"].(string)
	var rows []map[string]any
	require.NoError(t, json.Unmarshal([]byte(text), &rows))
	require.Len(t, rows, 1)
	assert.Equal(t, float64(1198), rows[0]["number"])
}

func TestGetPRDiffSummary(t *testing.T) {
	srv, _, _, out, in := newTestServer(t, false)
	resps := sendAndCollect(t, srv, out, in, []string{
		`{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"get_pr_diff","arguments":{"url":"https://github.com/acme/api/pull/1234","summary":true}}}`,
	})
	require.Len(t, resps, 1)
	text := resps[0]["result"].(map[string]any)["content"].([]any)[0].(map[string]any)["text"].(string)
	var summary map[string]any
	require.NoError(t, json.Unmarshal([]byte(text), &summary))
	assert.Equal(t, float64(1), summary["files"])
	assert.Equal(t, float64(2), summary["additions"])
	assert.Equal(t, float64(1), summary["deletions"])
}

func TestMutationsGate(t *testing.T) {
	// Without --allow-mutations, add_pr is not in tools/list and tools/call errors.
	srv, _, _, out, in := newTestServer(t, false)
	resps := sendAndCollect(t, srv, out, in, []string{
		`{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"add_pr","arguments":{"url":"https://github.com/acme/api/pull/9"}}}`,
	})
	require.Len(t, resps, 1)
	assert.NotNil(t, resps[0]["error"])
}

func TestMutationsAllowed(t *testing.T) {
	srv, _, s, out, in := newTestServer(t, true)
	resps := sendAndCollect(t, srv, out, in, []string{
		`{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"add_pr","arguments":{"url":"https://github.com/acme/api/pull/9999"}}}`,
	})
	require.Len(t, resps, 1)
	require.Nil(t, resps[0]["error"], "expected success, got: %v", resps[0]["error"])

	active, err := s.Active()
	require.NoError(t, err)
	assert.Equal(t, []string{"https://github.com/acme/api/pull/9999"}, active)
}

func TestUnknownMethod(t *testing.T) {
	srv, _, _, out, in := newTestServer(t, false)
	resps := sendAndCollect(t, srv, out, in, []string{
		`{"jsonrpc":"2.0","id":1,"method":"foo/bar"}`,
	})
	require.Len(t, resps, 1)
	require.NotNil(t, resps[0]["error"])
	assert.Equal(t, float64(-32601), resps[0]["error"].(map[string]any)["code"])
}
