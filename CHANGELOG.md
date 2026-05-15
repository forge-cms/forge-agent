# Changelog — forge-agent

## [0.3.2] — 2026-05-15

### Fixed

- `forge-agent/flow`: add `forge.MCP(forge.MCPRead, forge.MCPWrite)` to `forge.NewModule` — AgentJob MCP tools were not generated without this explicit option.

## [0.3.1] — 2026-05-15

### Changed

- Renamed sub-package `forge-agent/forge/` to `forge-agent/flow/`. Import path is now `forge-cms.dev/forge-agent/flow`. No logic changes.

## [0.3.0] — 2026-05-15

### Added

- `forge-agent/forge/` — new AGPL-3.0-or-later sub-package that integrates forge-agent with a Forge application.
- `AgentJob` — Forge content type (embeds `forge.Node`) with full lifecycle management: Draft → Published → Archived.
- `Module` — wires AgentJob into a Forge app via `Register(*forge.App)`. Registers MCP tools, subscribes to the signal bus, and manages the cron scheduler.
- `CreateTable(db)` — creates the `agent_jobs` SQL table.
- Auto-generated MCP tools: `create_agent_job`, `get_agent_job`, `list_agent_jobs`, `update_agent_job`, `publish_agent_job`, `archive_agent_job`, `delete_agent_job`.
- Signal-triggered jobs: any `forge.Signal` value as `Trigger`; `ContentTypeFilter` restricts to a content type.
- Cron-triggered jobs: 5-field cron expression as `Trigger`; scheduler rebuilds atomically on publish/archive.
- `WebhookURL` field: if set, agent task prompt includes an instruction to POST output via `http_post`.
- Guard: AgentJob lifecycle events never trigger other jobs (prevents self-activation loops).

## [0.2.2] — 2026-05-06

### Changed

- `cmd/scheduler`: cron schedule 06:00 → 13:45 (DK2 day-ahead prices available ~13:00 CET).
- API: `sort=HourUTC desc` — newest records first, avoids returning historical prices.
- System prompt: calendar-date grouping instead of fixed 24 h split.

## [0.2.1] — 2026-05-01

### Added

- SIGUSR1 run-now trigger in `cmd/scheduler` (Linux only; no-op on other platforms).
- Platform-specific build files: `sigusr1_linux.go` / `sigusr1_other.go`.

## [0.2.0] — 2026-04-28

### Added

- `Scheduler` type wrapping `gocron/v2` for cron-driven agent jobs.
- `Job` struct: `Schedule`, `Timezone`, `Task`, `Config`.
- `NewScheduler(jobs []Job)` — validates all timezones and cron expressions at startup.
- Singleton mode: overlapping runs are skipped, not queued.
- `cmd/scheduler` — UC2 binary: DK2 electricity prices → ntfy.sh (daily 06:00 CPH).

## [0.1.0] — 2026-04-01

### Added

- `Agent` type with Anthropic tool-use loop.
- `Config`: `MCPURL`, `MCPToken`, `SystemPrompt`, `Model`, `MaxTurns`, `StreamableHTTP`.
- MCP client: SSE (forge-mcp) and Streamable HTTP (GitHub MCP) transports.
- Built-in tools: `http_get`, `http_post`.
- `cmd/agent-forge` — forge-mcp agent binary.
- `cmd/agent-github` — GitHub MCP agent binary.
