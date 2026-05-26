// agent-github connects to the GitHub MCP server and summarises open issues.
//
// Required environment variables:
//
//	ANTHROPIC_API_KEY  Anthropic API key
//	GITHUB_TOKEN       GitHub personal access token (repo scope)
//	GITHUB_REPO        Repository in owner/repo format (e.g. forge-cms/forge)
//
// Optional environment variables:
//
//	GITHUB_MCP_URL     GitHub MCP endpoint (default: https://api.githubcopilot.com/mcp/)
package main

import (
	"context"
	"fmt"
	"os"

	"smeldr.dev/agent"
)

const defaultGitHubMCPURL = "https://api.githubcopilot.com/mcp/"

func main() {
	mcpURL := os.Getenv("GITHUB_MCP_URL")
	if mcpURL == "" {
		mcpURL = defaultGitHubMCPURL
	}

	repo := os.Getenv("GITHUB_REPO")
	if repo == "" {
		fmt.Fprintln(os.Stderr, "error: GITHUB_REPO is required (e.g. forge-cms/forge)")
		os.Exit(1)
	}

	cfg := agent.Config{
		MCPURL:         mcpURL,
		MCPToken:       os.Getenv("GITHUB_TOKEN"),
		StreamableHTTP: true, // GitHub MCP uses Streamable HTTP transport
		SystemPrompt:   "You are a helpful assistant with access to GitHub.",
	}

	a := agent.New(cfg)
	result, err := a.Run(context.Background(),
		fmt.Sprintf("List the open issues in the %s repository and summarize them.", repo))
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}
	fmt.Println(result)
}
