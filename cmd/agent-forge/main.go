// agent-forge connects to a forge-mcp server and summarises published posts.
//
// Required environment variables:
//
//	ANTHROPIC_API_KEY  Anthropic API key
//	FORGE_MCP_URL      forge-mcp SSE endpoint (e.g. https://example.com/mcp)
//	FORGE_TOKEN        forge-mcp Bearer token
package main

import (
	"context"
	"fmt"
	"os"

	agent "forge-cms.dev/forge-agent"
)

func main() {
	cfg := agent.Config{
		MCPURL:   os.Getenv("FORGE_MCP_URL"),
		MCPToken: os.Getenv("FORGE_TOKEN"),
		SystemPrompt: "You are a helpful assistant with access to a Forge CMS server. " +
			"Use the available MCP tools to answer questions about the site.",
	}

	a := agent.New(cfg)
	result, err := a.Run(context.Background(),
		"List all published posts and summarize the site in two sentences.")
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}
	fmt.Println(result)
}
