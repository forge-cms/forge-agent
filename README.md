# forge-agent

A minimal Go agent runtime: Anthropic API + tool use loop + MCP client + built-in HTTP tools. Ships with a cron scheduler for running agent jobs 24/7 in the cloud.

forge-agent connects Claude to any MCP server via SSE or Streamable HTTP, dispatches tool calls, and drives the conversation to completion. It ships as a library (`smeldr.dev/agent`) and runnable binaries.

**Latest: v0.5.1** — [smeldr.dev/agent/flow](https://pkg.go.dev/smeldr.dev/agent/flow) (AGPL) · [smeldr.dev/agent](https://pkg.go.dev/smeldr.dev/agent) (MIT)

---

## Dependencies

| Package | Purpose |
|---------|---------|
| `github.com/anthropics/anthropic-sdk-go` | Anthropic API and tool use protocol |
| `github.com/modelcontextprotocol/go-sdk` | Official MCP Go SDK (Apache 2.0) |
| `github.com/go-co-op/gocron/v2` | Cron scheduler (Apache 2.0) |

The MCP SDK is maintained by the MCP organization. Spec changes are tracked automatically — forge-agent does not maintain its own MCP transport layer.

gocron is used instead of stdlib `time` + goroutines because timezone handling on Alpine/Linux servers is a known failure mode with plain goroutine-based schedulers (forge-social hit this in v0.4.0), and missed job recovery on restart requires non-trivial handling that gocron provides out of the box.

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

## Examples

| Example | Description |
|---------|-------------|
| [`example/electricity-advisor/`](example/electricity-advisor/) | A daily DK2 electricity price advisor that posts cheap-window recommendations to ntfy.sh — demonstrates `agent.NewScheduler` with a cron-triggered Job. |

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

## Scheduler

forge-agent ships a cron scheduler for running agent jobs continuously in the cloud.

### Job and Scheduler

```go
type Job struct {
    Schedule string // 5-field cron expression (e.g. "0 6 * * *")
    Timezone string // IANA timezone (e.g. "Europe/Copenhagen"); empty = UTC
    Task     string // prompt passed to Agent.Run on each execution
    Config   Config // agent config for this job
}

s, err := agent.NewScheduler([]agent.Job{
    {
        Schedule: "0 6 * * *",
        Timezone: "Europe/Copenhagen",
        Task:     "Fetch electricity prices and post a recommendation.",
        Config:   agent.Config{SystemPrompt: "..."},
    },
})
if err != nil {
    log.Fatal(err)
}
s.Start()
defer s.Stop()
```

- `NewScheduler` validates all timezones and cron expressions at startup — fail-fast, not at first run.
- Each job runs in singleton mode: if a job is still running when its next trigger fires, the new run is skipped.
- Missed jobs on restart are not caught up — the next scheduled run fires as normal.
- `Stop` blocks until all in-flight jobs complete (graceful shutdown).

### `time/tzdata` embed

The `example/electricity-advisor` binary embeds the Go timezone database with `import _ "time/tzdata"`. This is required on Alpine and scratch containers that have no OS-level tzdata. The library itself does not embed it — callers who manage tzdata themselves are not affected.

### Quick start — `example/electricity-advisor`

```bash
export ANTHROPIC_API_KEY=sk-ant-...
export NTFY_TOPIC=my-ntfy-topic

go run ./example/electricity-advisor
```

The scheduler fires at 06:00 Europe/Copenhagen each day, fetches 48 hours of DK2 electricity spot prices, identifies the cheapest 2-hour window in the next 24 hours and in the following 24 hours, and posts a concise recommendation in Danish to `https://ntfy.sh/$NTFY_TOPIC`.

### Systemd deploy on Hetzner (linux/amd64)

**1. Cross-compile**

```powershell
$env:GOOS = "linux"; $env:GOARCH = "amd64"
go build -o forge-agent-scheduler ./example/electricity-advisor
$env:GOOS = ""; $env:GOARCH = ""
```

**2. Copy to server**

```bash
scp forge-agent-scheduler root@your-server:/usr/local/bin/
scp example/electricity-advisor/deploy/forge-agent-scheduler.service root@your-server:/etc/systemd/system/
```

**3. Create the env file on the server**

```bash
mkdir -p /etc/forge-agent
cat > /etc/forge-agent/scheduler.env <<EOF
ANTHROPIC_API_KEY=sk-ant-...
NTFY_TOPIC=my-ntfy-topic
EOF
chmod 600 /etc/forge-agent/scheduler.env
```

**4. Install and start the service**

```bash
systemctl daemon-reload
systemctl enable forge-agent-scheduler
systemctl start forge-agent-scheduler
systemctl status forge-agent-scheduler
```

### Triggering a manual run

Send SIGUSR1 to trigger an immediate agent run without restarting the service:

```bash
systemctl kill -s SIGUSR1 forge-agent-scheduler
```

The service continues running normally after the triggered run completes.

---

## Forge integration — `smeldr.dev/agent/flow`

`smeldr.dev/agent/flow` is an AGPL-3.0-or-later sub-package that wires
agent execution into a Forge application. It exposes `AgentJob` as a Forge content
type with full lifecycle management and auto-generated MCP tools.

### Quick start

```go
import forgeagent "smeldr.dev/agent/flow"

// At startup — create table before connecting the module.
forgeagent.CreateTable(db)

agentMod := forgeagent.New(db, forgeagent.Config{
    MCPURL:   "http://localhost:8080/mcp",
    MCPToken: os.Getenv("FORGE_TOKEN"),
})
agentMod.Register(app) // wires MCP tools + signal bus
defer agentMod.Stop()
```

### AgentJob fields

| Field | Type | Description |
|-------|------|-------------|
| `Name` | string | Human-readable identifier. Used as slug source. Required. |
| `Trigger` | string | 5-field cron expression (`"45 13 * * *"`) or forge signal name (`"after_publish"`). Required. |
| `ContentTypeFilter` | string | Restrict signal triggers to a content type (e.g. `"Post"`). Empty = all types. Ignored for cron triggers. |
| `SystemPrompt` | string | System instruction prepended to every run. Required. |
| `Model` | string | Anthropic model ID. Defaults to `"claude-sonnet-4-6"`. |
| `MaxTurns` | int | Max tool-use loops. Defaults to 10. |
| `WebhookURL` | string | If set, agent's task prompt includes an instruction to POST output here via `http_post`. |

Status lifecycle: Draft (job exists, does not run) → Published (active) → Archived (stopped).

### MCP tools (auto-generated)

`create_agent_job`, `get_agent_job`, `list_agent_jobs`, `update_agent_job`,
`publish_agent_job`, `archive_agent_job`, `delete_agent_job`. Role: Admin.

### Signal triggers

Set `Trigger` to any `smeldr.Signal` string value:

| Signal | Fires when |
|--------|-----------|
| `after_publish` | Content transitions to Published |
| `after_create` | New content item created |
| `after_update` | Content updated |
| `after_unpublish` | Content moved out of Published |
| `after_archive` | Content archived |
| `after_schedule` | Content scheduled |
| `after_delete` | Content deleted |

Set `ContentTypeFilter` to restrict to a content type (e.g. `"Post"`). Leave empty
to match all types. Note: AgentJob lifecycle events never trigger other jobs — the
module guards against self-activation automatically.

### UC1 — devlog-social-drafts example

```
1. create_agent_job — name: "devlog-social-drafts",
                      trigger: "after_publish",
                      content_type_filter: "Post",
                      system_prompt: "Draft LinkedIn and X posts for this content."
2. publish_agent_job slug="devlog-social-drafts" — activates the job
3. Publish a Post — signal fires, agent runs, creates scheduled social posts
4. archive_agent_job slug="devlog-social-drafts" — deactivates the job
```

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
