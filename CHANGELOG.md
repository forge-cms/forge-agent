# Changelog — forge-agent

## [0.3.6] — 2026-05-17

### Added

- `flow/module.go`: `slog.Info` at each key point in `handleSignal` — entry,
  jobs loaded count, per-job match evaluation, goroutine spawn, and run completion.
- `agent.go`: `slog.Info` before/after `connectMCP`, at the start of each
  Anthropic API turn, after each response, and before each tool dispatch.
  All log lines use structured key/value fields for easy filtering.

---

## [0.3.5] — 2026-05-17

### Fixed

- `agent`: `connectMCP` now calls `context.WithoutCancel` before establishing the SSE or Streamable HTTP connection. Signal-triggered jobs previously failed with `context canceled` because `forge.App.dispatchBus` applies a 100 ms deadline to each `OnSignal` handler and calls `cancel()` immediately after it returns. The partial fix in `forge-agent/flow` (v0.3.4) detached the goroutine's context but left `connectMCP` itself vulnerable to any short-lived parent context at any call site. Detaching inside `connectMCP` is the correct layer: the SSE stream lifetime is an implementation detail of the MCP connection, not the caller's responsibility.

## [0.3.4] — 2026-05-15

### Fixed

- `forge-agent/flow`: add `json` tags to `AgentJob` fields whose Go names differ from their snake_case JSON keys (`content_type_filter`, `system_prompt`, `max_turns`, `webhook_url`). Without these tags `json.Unmarshal` silently dropped the values, causing `create_agent_job` via MCP to return a spurious `validation failed: SystemPrompt: required` error even when `system_prompt` was supplied.

## [0.3.3] — 2026-05-15

### Fixed

- `forge-agent/flow`: signal-triggered jobs now receive an enriched task string with the full `forge.SignalEvent` serialised as JSON. Previously the agent received only the static cron prompt with no information about what triggered it.

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
