// AGPL-3.0-or-later

// Package forgeagent wires forge-agent into a Forge application.
// It exposes [AgentJob] as a Forge content type, which gives it full
// lifecycle management (Draft → Published → Archived) and auto-generated
// MCP tools (create_agent_job, get_agent_job, list_agent_jobs,
// update_agent_job, publish_agent_job, archive_agent_job, delete_agent_job).
//
// Usage:
//
//	db := smeldr.OpenDB("forge.db")
//	forgeagent.CreateTable(db)
//
//	agentMod := forgeagent.New(db, forgeagent.Config{
//	    MCPURL:   "http://localhost:8080/mcp",
//	    MCPToken: os.Getenv("FORGE_TOKEN"),
//	})
//	agentMod.Register(app)
//	defer agentMod.Stop()
package forgeagent

import (
	"strings"

	"smeldr.dev/core"
)

// AgentJob is a Forge Node module representing a scheduled or signal-triggered
// agent task. Published jobs are active; Draft and Archived jobs do not run.
//
// Trigger is either a 5-field cron expression (e.g. "45 13 * * *") or the
// string value of a [smeldr.Signal] constant (e.g. "after_publish"). The
// distinction is by whitespace: cron expressions contain spaces; signal names
// do not.
//
// Set ContentTypeFilter to restrict a signal-triggered job to a specific
// content type (e.g. "Post"). An empty ContentTypeFilter matches all content
// types — including other AgentJobs. Use ContentTypeFilter whenever the trigger
// is a content lifecycle signal to avoid unintended cross-job activation.
type AgentJob struct {
	smeldr.Node
	// Name is the human-readable identifier for this job. Used as the slug source.
	Name string `forge:"required"`
	// Trigger is a 5-field cron expression or a smeldr.Signal string value.
	Trigger string `forge:"required"`
	// ContentTypeFilter restricts signal-triggered jobs to the named content type.
	// Empty matches all types. Ignored for cron triggers.
	ContentTypeFilter string `db:"content_type_filter" json:"content_type_filter"`
	// SystemPrompt is the agent's system instruction, prepended to every run.
	SystemPrompt string `forge:"required" db:"system_prompt" json:"system_prompt"`
	// Model is the Anthropic model ID. Defaults to "claude-sonnet-4-6" when empty.
	Model string
	// MaxTurns is the maximum tool-use loops per run. Defaults to 10 when zero.
	MaxTurns int `db:"max_turns" json:"max_turns"`
	// WebhookURL is an optional output channel. When set, the agent's task
	// prompt includes an instruction to POST output here via http_post.
	WebhookURL string `db:"webhook_url" json:"webhook_url"`
}

// ContentTitle implements [smeldr.Titled] so signal events carry the job name.
func (j *AgentJob) ContentTitle() string { return j.Name }

// isCronTrigger returns true when Trigger is a cron expression.
// Cron expressions contain spaces ("45 13 * * *"); signal names do not ("after_publish").
func (j *AgentJob) isCronTrigger() bool {
	return strings.ContainsRune(j.Trigger, ' ')
}
