// Package agent provides a minimal agent execution runtime: Anthropic API +
// tool use loop + MCP client + built-in HTTP tools.
package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	anthropic "github.com/anthropics/anthropic-sdk-go"
	mcpsdk "github.com/modelcontextprotocol/go-sdk/mcp"
)

// mcpClient wraps a connected MCP client session and its tool snapshot.
type mcpClient struct {
	session *mcpsdk.ClientSession
	tools   []*mcpsdk.Tool
}

// bearerTransport injects Authorization: Bearer <token> into every request.
type bearerTransport struct {
	token string
	base  http.RoundTripper
}

func (t *bearerTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	clone := req.Clone(req.Context())
	clone.Header.Set("Authorization", "Bearer "+t.token)
	base := t.base
	if base == nil {
		base = http.DefaultTransport
	}
	return base.RoundTrip(clone)
}

// connectMCP connects to an MCP server and snapshots its tool list.
// Set streamable=true for Streamable HTTP transport (GitHub MCP, 2025-03-26+ spec).
// Set streamable=false for SSE transport (forge-mcp, 2024-11-05 spec).
func connectMCP(ctx context.Context, serverURL, token string, streamable bool) (*mcpClient, error) {
	// Detach from any parent cancellation so the SSE stream outlives the
	// triggering HTTP request, signal bus deadline, or other short-lived context.
	ctx = context.WithoutCancel(ctx)

	var httpClient *http.Client
	if token != "" {
		httpClient = &http.Client{Transport: &bearerTransport{token: token}}
	}

	var transport mcpsdk.Transport
	if streamable {
		transport = &mcpsdk.StreamableClientTransport{
			Endpoint:   serverURL,
			HTTPClient: httpClient,
		}
	} else {
		transport = &mcpsdk.SSEClientTransport{
			Endpoint:   serverURL,
			HTTPClient: httpClient,
		}
	}

	client := mcpsdk.NewClient(&mcpsdk.Implementation{
		Name:    "forge-agent",
		Version: "v0.1.0",
	}, nil)

	session, err := client.Connect(ctx, transport, nil)
	if err != nil {
		return nil, fmt.Errorf("mcp connect: %w", err)
	}

	res, err := session.ListTools(ctx, nil)
	if err != nil {
		session.Close()
		return nil, fmt.Errorf("mcp list tools: %w", err)
	}

	return &mcpClient{session: session, tools: res.Tools}, nil
}

func (c *mcpClient) close() {
	c.session.Close()
}

// toolParams converts the MCP tool snapshot to Anthropic ToolUnionParam slice.
func (c *mcpClient) toolParams() []anthropic.ToolUnionParam {
	params := make([]anthropic.ToolUnionParam, 0, len(c.tools))
	for _, t := range c.tools {
		var schema anthropic.ToolInputSchemaParam
		if t.InputSchema != nil {
			// Round-trip via JSON: MCP and Anthropic schema types use different
			// internal representations but compatible wire formats.
			if b, err := json.Marshal(t.InputSchema); err == nil {
				_ = json.Unmarshal(b, &schema)
			}
		}
		p := anthropic.ToolParam{
			Name:        t.Name,
			InputSchema: schema,
		}
		if t.Description != "" {
			p.Description = anthropic.String(t.Description)
		}
		params = append(params, anthropic.ToolUnionParam{OfTool: &p})
	}
	return params
}

// call dispatches a named tool call with JSON-encoded arguments to the MCP server.
// Returns (result text, isError).
func (c *mcpClient) call(ctx context.Context, name string, rawInput json.RawMessage) (string, bool, error) {
	var args map[string]any
	if err := json.Unmarshal(rawInput, &args); err != nil {
		return "", true, fmt.Errorf("unmarshal tool args: %w", err)
	}

	result, err := c.session.CallTool(ctx, &mcpsdk.CallToolParams{
		Name:      name,
		Arguments: args,
	})
	if err != nil {
		return "", true, err
	}

	var parts []string
	for _, content := range result.Content {
		if tc, ok := content.(*mcpsdk.TextContent); ok {
			parts = append(parts, tc.Text)
		}
	}
	return strings.Join(parts, "\n"), result.IsError, nil
}

// hasTool reports whether the MCP server exposes a tool with the given name.
func (c *mcpClient) hasTool(name string) bool {
	for _, t := range c.tools {
		if t.Name == name {
			return true
		}
	}
	return false
}
