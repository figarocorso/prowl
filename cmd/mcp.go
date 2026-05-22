package cmd

import (
	"context"
	"os"
	"strconv"

	"github.com/figarocorso/prowl/internal/mcp"
	"github.com/spf13/cobra"
)

var mcpAllowMutations bool

var mcpCmd = &cobra.Command{
	Use:   "mcp",
	Short: "Run a stdio MCP server exposing tracked PRs to AI agents",
	Long: `mcp starts a Model Context Protocol server on stdio so AI agents
(Claude Code, Claude Desktop, ...) can query tracked PRs through tools:

  list_prs       — filter by source / status / assignee
  get_pr         — full detail for a single PR
  get_pr_diff    — unified diff (or summary)

Mutating tools (add_pr / remove_pr) are gated behind --allow-mutations or the
PROWL_MCP_ALLOW_MUTATIONS env var.`,
	RunE: runMCP,
}

func init() {
	mcpCmd.Flags().BoolVar(&mcpAllowMutations, "allow-mutations", false, "expose add_pr / remove_pr tools (default off)")
	rootCmd.AddCommand(mcpCmd)
}

func runMCP(_ *cobra.Command, _ []string) error {
	cfg, s, err := loadConfigAndStore()
	if err != nil {
		return err
	}
	client, err := clientFactory()
	if err != nil {
		return err
	}
	allow := mcpAllowMutations
	if v := os.Getenv("PROWL_MCP_ALLOW_MUTATIONS"); v != "" {
		if b, err := strconv.ParseBool(v); err == nil {
			allow = b
		}
	}
	srv := mcp.NewServer(mcp.Options{
		Config:         cfg,
		Store:          s,
		Client:         client,
		AllowMutations: allow,
		Version:        buildVersion,
	})
	return srv.Run(context.Background())
}
