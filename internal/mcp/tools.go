package mcp

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/figarocorso/prowl/internal/data"
)

type toolDef struct {
	Name        string         `json:"name"`
	Description string         `json:"description"`
	InputSchema map[string]any `json:"inputSchema"`
}

func (s *Server) toolDefinitions() []toolDef {
	defs := []toolDef{
		{
			Name:        "list_prs",
			Description: "List tracked PRs with current GitHub state. Filter by source (active|reviewed|all), status (open|closed|merged|blocked|draft), or assignee.",
			InputSchema: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"source":   map[string]any{"type": "string", "enum": []string{"active", "reviewed", "all"}, "default": "active"},
					"status":   map[string]any{"type": "string", "description": "filter by computed status label (open|open/blocked|draft|merged|closed)"},
					"assignee": map[string]any{"type": "string", "description": "GitHub login to filter by"},
				},
			},
		},
		{
			Name:        "get_pr",
			Description: "Fetch full detail (state, assignees, queue position) for a single PR by URL.",
			InputSchema: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"url": map[string]any{"type": "string", "description": "PR URL"},
				},
				"required": []string{"url"},
			},
		},
		{
			Name:        "get_pr_diff",
			Description: "Fetch the unified diff for a PR. Pass summary=true for a {files,additions,deletions} digest instead of the full diff.",
			InputSchema: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"url":     map[string]any{"type": "string"},
					"summary": map[string]any{"type": "boolean", "default": false},
				},
				"required": []string{"url"},
			},
		},
	}
	if s.allowMutations {
		defs = append(defs,
			toolDef{
				Name:        "add_pr",
				Description: "Start tracking a PR URL. Gated by PROWL_MCP_ALLOW_MUTATIONS.",
				InputSchema: map[string]any{
					"type":       "object",
					"properties": map[string]any{"url": map[string]any{"type": "string"}},
					"required":   []string{"url"},
				},
			},
			toolDef{
				Name:        "remove_pr",
				Description: "Stop tracking a PR. Gated by PROWL_MCP_ALLOW_MUTATIONS.",
				InputSchema: map[string]any{
					"type":       "object",
					"properties": map[string]any{"url": map[string]any{"type": "string"}},
					"required":   []string{"url"},
				},
			},
		)
	}
	return defs
}

type toolCallParams struct {
	Name      string         `json:"name"`
	Arguments map[string]any `json:"arguments"`
}

type toolResult struct {
	Content []toolContent `json:"content"`
	IsError bool          `json:"isError,omitempty"`
}

type toolContent struct {
	Type string `json:"type"`
	Text string `json:"text"`
}

func textResult(s string) toolResult {
	return toolResult{Content: []toolContent{{Type: "text", Text: s}}}
}

func jsonResult(v any) toolResult {
	b, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return toolResult{Content: []toolContent{{Type: "text", Text: err.Error()}}, IsError: true}
	}
	return toolResult{Content: []toolContent{{Type: "text", Text: string(b)}}}
}

func (s *Server) handleToolCall(ctx context.Context, params json.RawMessage) (toolResult, error) {
	var call toolCallParams
	if err := json.Unmarshal(params, &call); err != nil {
		return toolResult{}, fmt.Errorf("parse params: %w", err)
	}
	switch call.Name {
	case "list_prs":
		return s.toolListPRs(ctx, call.Arguments)
	case "get_pr":
		return s.toolGetPR(ctx, call.Arguments)
	case "get_pr_diff":
		return s.toolGetPRDiff(ctx, call.Arguments)
	case "add_pr":
		if !s.allowMutations {
			return toolResult{}, fmt.Errorf("mutations are disabled")
		}
		return s.toolAddPR(call.Arguments)
	case "remove_pr":
		if !s.allowMutations {
			return toolResult{}, fmt.Errorf("mutations are disabled")
		}
		return s.toolRemovePR(call.Arguments)
	default:
		return toolResult{}, fmt.Errorf("unknown tool: %s", call.Name)
	}
}

func stringArg(args map[string]any, key string) string {
	v, _ := args[key].(string)
	return v
}

func boolArg(args map[string]any, key string) bool {
	v, _ := args[key].(bool)
	return v
}

func (s *Server) toolListPRs(ctx context.Context, args map[string]any) (toolResult, error) {
	urls, err := s.listSourceURLs(stringArg(args, "source"))
	if err != nil {
		return toolResult{}, err
	}
	results := s.client.FetchBatch(ctx, urls)
	rows := filterPRRows(results, stringArg(args, "status"), stringArg(args, "assignee"))
	return jsonResult(rows), nil
}

func (s *Server) listSourceURLs(source string) ([]string, error) {
	source = strings.ToLower(source)
	if source == "" {
		source = "active"
	}
	switch source {
	case "active":
		return s.store.Active()
	case "reviewed":
		return s.store.Reviewed()
	case "all":
		a, err := s.store.Active()
		if err != nil {
			return nil, err
		}
		r, err := s.store.Reviewed()
		if err != nil {
			return nil, err
		}
		return append(a, r...), nil
	default:
		return nil, fmt.Errorf("invalid source %q", source)
	}
}

func filterPRRows(results []data.Result, statusFilter, assigneeFilter string) []map[string]any {
	statusFilter = strings.ToLower(statusFilter)
	rows := make([]map[string]any, 0, len(results))
	for _, r := range results {
		if row, ok := buildPRRow(r, statusFilter, assigneeFilter); ok {
			rows = append(rows, row)
		}
	}
	return rows
}

func buildPRRow(r data.Result, statusFilter, assigneeFilter string) (map[string]any, bool) {
	row := map[string]any{"url": r.URL}
	if r.Err != nil {
		row["error"] = r.Err.Error()
		return row, true
	}
	pr := r.PR
	status := data.StatusLabel(pr)
	if statusFilter != "" && !strings.EqualFold(status, statusFilter) {
		return nil, false
	}
	if assigneeFilter != "" && !containsFold(pr.Assignees, assigneeFilter) {
		return nil, false
	}
	row["number"] = pr.Number
	row["title"] = pr.Title
	row["state"] = pr.State
	row["status"] = status
	row["is_draft"] = pr.IsDraft
	row["assignees"] = pr.Assignees
	if pr.Queue != nil {
		row["queue_state"] = pr.Queue.State
		row["queue_position"] = pr.Queue.Position
		if pr.Queue.ETA > 0 {
			row["queue_eta_seconds"] = int(pr.Queue.ETA.Seconds())
		}
	}
	return row, true
}

func (s *Server) toolGetPR(ctx context.Context, args map[string]any) (toolResult, error) {
	url := stringArg(args, "url")
	if url == "" {
		return toolResult{}, fmt.Errorf("url is required")
	}
	canonical, err := data.CanonicalURL(url)
	if err != nil {
		return toolResult{}, err
	}
	pr, err := s.client.Fetch(ctx, canonical)
	if err != nil {
		return toolResult{}, err
	}
	return jsonResult(pr), nil
}

func (s *Server) toolGetPRDiff(ctx context.Context, args map[string]any) (toolResult, error) {
	url := stringArg(args, "url")
	if url == "" {
		return toolResult{}, fmt.Errorf("url is required")
	}
	canonical, err := data.CanonicalURL(url)
	if err != nil {
		return toolResult{}, err
	}
	diff, err := s.diffFetcher(ctx, canonical)
	if err != nil {
		return toolResult{}, err
	}
	if boolArg(args, "summary") {
		return jsonResult(data.SummarizeDiff(diff)), nil
	}
	return textResult(diff), nil
}

func (s *Server) toolAddPR(args map[string]any) (toolResult, error) {
	url := stringArg(args, "url")
	if url == "" {
		return toolResult{}, fmt.Errorf("url is required")
	}
	canonical, err := data.CanonicalURL(url)
	if err != nil {
		return toolResult{}, err
	}
	added, err := s.store.Add(canonical)
	if err != nil {
		return toolResult{}, err
	}
	return jsonResult(map[string]any{"url": canonical, "added": added}), nil
}

func (s *Server) toolRemovePR(args map[string]any) (toolResult, error) {
	url := stringArg(args, "url")
	if url == "" {
		return toolResult{}, fmt.Errorf("url is required")
	}
	canonical, err := data.CanonicalURL(url)
	if err != nil {
		return toolResult{}, err
	}
	n, err := s.store.Remove(canonical)
	if err != nil {
		return toolResult{}, err
	}
	return jsonResult(map[string]any{"url": canonical, "removed_rows": n}), nil
}

func containsFold(haystack []string, needle string) bool {
	for _, h := range haystack {
		if strings.EqualFold(h, needle) {
			return true
		}
	}
	return false
}
