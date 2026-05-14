# forge-agent

A minimal Go agent runtime: Anthropic API + tool use loop + MCP client + built-in HTTP tools.

forge-agent connects Claude to any MCP server via SSE or Streamable HTTP, dispatches tool calls, and drives the conversation to completion. It ships as a library (`forge-cms.dev/forge-agent`) and two runnable example binaries.

**v0.1.0 — Agent loop core**

---

## Dependencies

Two third-party dependencies only:

| Package | Purpose |
|---------|---------|
| `github.com/anthropics/anthropic-sdk-go` | Anthropic API and tool use protocol |
| `github.com/modelcontextprotocol/go-sdk` | Official MCP Go SDK (Apache 2.0) |

The MCP SDK is maintained by the MCP organization. Spec changes are tracked automatically — forge-agent does not maintain its own MCP transport layer.

---

## Quick start

```bash
export ANTHROPIC_API_KEY=sk-ant-...
export FORGE_MCP_URL=https://your-site.example.com/mcp
export FORGE_TOKEN=your-forge-token

go run ./cmd/agent-forge
```

For GitHub:

```bash
export ANTHROPIC_API_KEY=sk-ant-...
export GITHUB_TOKEN=ghp_...
export GITHUB_REPO=forge-cms/forge

go run ./cmd/agent-github
```

---

## Config reference

```go
type Config struct {
    MCPURL         string // MCP server endpoint; empty = no MCP tools
    MCPToken       string // Bearer token for the MCP server
    SystemPrompt   string // System message prepended to every conversation
    Model          string // Anthropic model ID (default: "claude-sonnet-4-6")
    MaxTurns       int    // Max tool-use loops before giving up (default: 10)
    StreamableHTTP bool   // Use Streamable HTTP transport instead of SSE
}
```

Set `StreamableHTTP: true` when connecting to the GitHub MCP server or any server
implementing the 2025-03-26+ spec. Leave it `false` (the default) for forge-mcp,
which uses the SSE transport from the 2024-11-05 spec.

---

## Built-in tools

Two tools are always available in every agent run, alongside MCP tools:

### `http_get`

```json
{
  "name": "http_get",
  "input": { "url": "https://api.example.com/data" }
}
```

Makes an HTTP GET request. Returns the response body (capped at 32 KB).
On non-2xx: returns `"HTTP <status>: <body prefix>"`.

### `http_post`

```json
{
  "name": "http_post",
  "input": {
    "url": "https://ntfy.sh/my-topic",
    "body": "Electricity is cheap between 02:00 and 06:00.",
    "content_type": "text/plain"
  }
}
```

Makes an HTTP POST request. `content_type` defaults to `"text/plain"`.
Use `"application/json"` for Discord webhooks and JSON APIs.
Returns `"HTTP <status>: <response body prefix>"`.

---

## Architecture note

The MCP client in forge-agent is generic. It speaks to any SSE or Streamable HTTP
MCP server — not just forge-mcp. The `cmd/agent-github` binary demonstrates this:
it connects to the GitHub MCP server using Streamable HTTP while `cmd/agent-forge`
connects to forge-mcp using SSE. Same agent loop, different transport, different
tool catalog.

---

## License

MIT — see [LICENSE](LICENSE).
