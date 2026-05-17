package agent

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"

	anthropic "github.com/anthropics/anthropic-sdk-go"
)

// ErrMaxTurns is returned when the agent exhausts MaxTurns without reaching end_turn.
var ErrMaxTurns = errors.New("agent: max turns reached")

// Config holds the settings for an Agent run.
type Config struct {
	// MCPURL is the MCP server endpoint. If empty, no MCP tools are available.
	MCPURL string
	// MCPToken is the Bearer token sent to the MCP server.
	MCPToken string
	// SystemPrompt is prepended to every conversation as the system message.
	SystemPrompt string
	// Model is the Anthropic model ID. Defaults to "claude-sonnet-4-6".
	Model string
	// MaxTurns is the maximum number of tool-use loops before giving up. Defaults to 10.
	MaxTurns int
	// StreamableHTTP instructs the MCP client to use Streamable HTTP transport
	// instead of SSE. Use this when connecting to GitHub MCP or any 2025-03-26+ server.
	StreamableHTTP bool
}

func (c *Config) setDefaults() {
	if c.Model == "" {
		c.Model = "claude-sonnet-4-6"
	}
	if c.MaxTurns <= 0 {
		c.MaxTurns = 10
	}
}

// Agent drives an Anthropic tool-use loop with optional MCP and built-in tools.
type Agent struct {
	cfg Config
}

// New creates a new Agent with defaults applied.
func New(cfg Config) *Agent {
	cfg.setDefaults()
	return &Agent{cfg: cfg}
}

// Run connects to the MCP server (if MCPURL is set), then drives a tool-use loop
// until the model returns end_turn or MaxTurns is reached.
// The ANTHROPIC_API_KEY environment variable must be set.
func (a *Agent) Run(ctx context.Context, task string) (string, error) {
	var mcp *mcpClient
	if a.cfg.MCPURL != "" {
		slog.Info("agent: connecting to MCP",
			"url", a.cfg.MCPURL, "token_set", a.cfg.MCPToken != "")
		var err error
		mcp, err = connectMCP(ctx, a.cfg.MCPURL, a.cfg.MCPToken, a.cfg.StreamableHTTP)
		if err != nil {
			return "", fmt.Errorf("agent: %w", err)
		}
		defer mcp.close()
		slog.Info("agent: MCP connected", "tools", len(mcp.toolParams()))
	}

	bt := builtinTools()
	tools := make([]anthropic.ToolUnionParam, 0, len(bt))
	for _, b := range bt {
		tools = append(tools, b.param)
	}
	if mcp != nil {
		tools = append(tools, mcp.toolParams()...)
	}

	client := anthropic.NewClient()
	messages := []anthropic.MessageParam{
		{
			Role: anthropic.MessageParamRoleUser,
			Content: []anthropic.ContentBlockParamUnion{{
				OfText: &anthropic.TextBlockParam{Text: task},
			}},
		},
	}

	params := anthropic.MessageNewParams{
		Model:     anthropic.Model(a.cfg.Model),
		MaxTokens: 4096,
		Tools:     tools,
	}
	if a.cfg.SystemPrompt != "" {
		params.System = []anthropic.TextBlockParam{{Text: a.cfg.SystemPrompt}}
	}

	var lastText string
	for i := range a.cfg.MaxTurns {
		slog.Info("agent: calling Anthropic API", "turn", i+1)
		params.Messages = messages

		resp, err := client.Messages.New(ctx, params)
		if err != nil {
			return lastText, fmt.Errorf("agent: messages.new: %w", err)
		}

		for _, block := range resp.Content {
			if tb, ok := block.AsAny().(anthropic.TextBlock); ok && tb.Text != "" {
				lastText = tb.Text
			}
		}

		slog.Info("agent: Anthropic response",
			"stop_reason", string(resp.StopReason), "text_len", len(lastText))

		if resp.StopReason != anthropic.StopReasonToolUse {
			return lastText, nil
		}

		messages = append(messages, resp.ToParam())

		var toolResults []anthropic.ContentBlockParamUnion
		for _, block := range resp.Content {
			use, ok := block.AsAny().(anthropic.ToolUseBlock)
			if !ok {
				continue
			}
			slog.Info("agent: dispatching tool", "tool", use.Name)
			result, isErr := dispatchTool(ctx, use, bt, mcp)
			toolResults = append(toolResults, anthropic.ContentBlockParamUnion{
				OfToolResult: &anthropic.ToolResultBlockParam{
					ToolUseID: use.ID,
					Content: []anthropic.ToolResultBlockParamContentUnion{{
						OfText: &anthropic.TextBlockParam{Text: result},
					}},
					IsError: anthropic.Bool(isErr),
				},
			})
		}

		messages = append(messages, anthropic.MessageParam{
			Role:    anthropic.MessageParamRoleUser,
			Content: toolResults,
		})
	}

	return lastText, ErrMaxTurns
}

// dispatchTool routes a tool_use block to the correct handler.
// Returns (result text, isError).
func dispatchTool(ctx context.Context, use anthropic.ToolUseBlock, bt []builtinTool, mcp *mcpClient) (string, bool) {
	for _, b := range bt {
		if b.param.OfTool.Name == use.Name {
			var input map[string]any
			if err := json.Unmarshal(use.Input, &input); err != nil {
				return fmt.Sprintf("failed to parse tool input: %v", err), true
			}
			result, err := b.handler(ctx, input)
			if err != nil {
				return err.Error(), true
			}
			return result, false
		}
	}

	if mcp != nil && mcp.hasTool(use.Name) {
		result, isErr, err := mcp.call(ctx, use.Name, use.Input)
		if err != nil {
			return err.Error(), true
		}
		return result, isErr
	}

	return fmt.Sprintf("unknown tool: %s", use.Name), true
}
