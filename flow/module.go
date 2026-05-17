// AGPL-3.0-or-later

package forgeagent

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"sync"

	"forge-cms.dev/forge"
	agent "forge-cms.dev/forge-agent"
)

// Config holds the connection settings passed to every agent run.
type Config struct {
	// MCPURL is the forge-mcp endpoint (e.g. "http://localhost:8080/mcp").
	MCPURL string
	// MCPToken is a forge token with Author-or-higher role for agent MCP calls.
	MCPToken string
	// StreamableHTTP switches the MCP client from SSE to Streamable HTTP transport.
	// Use true when connecting to forge-mcp; false for SSE-only servers.
	StreamableHTTP bool
}

// Module integrates forge-agent with a Forge application.
// It registers [AgentJob] as a content type, subscribes to the signal bus,
// and manages a gocron scheduler for cron-triggered jobs.
//
// Call [New] to create the module, [Module.Register] to wire it into the app,
// and [Module.Stop] in the application's shutdown handler.
type Module struct {
	cfg   Config
	repo  forge.Repository[*AgentJob]
	mod   *forge.Module[*AgentJob]
	mu    sync.Mutex
	sched *agent.Scheduler
}

// CreateTable creates the agent_jobs table if it does not already exist.
// Call this once at application startup before [New].
func CreateTable(db forge.DB) error {
	_, err := db.ExecContext(context.Background(), `
		CREATE TABLE IF NOT EXISTS agent_jobs (
			id                  TEXT NOT NULL PRIMARY KEY,
			slug                TEXT NOT NULL UNIQUE,
			status              TEXT NOT NULL DEFAULT 'draft',
			published_at        DATETIME,
			scheduled_at        DATETIME,
			created_at          DATETIME NOT NULL,
			updated_at          DATETIME NOT NULL,
			name                TEXT NOT NULL DEFAULT '',
			trigger             TEXT NOT NULL DEFAULT '',
			content_type_filter TEXT NOT NULL DEFAULT '',
			system_prompt       TEXT NOT NULL DEFAULT '',
			model               TEXT NOT NULL DEFAULT '',
			max_turns           INTEGER NOT NULL DEFAULT 0,
			webhook_url         TEXT NOT NULL DEFAULT ''
		)`)
	return err
}

// New creates a Module backed by db with the given agent Config.
// Call [CreateTable] before New so the agent_jobs table exists.
func New(db forge.DB, cfg Config) *Module {
	return newWithRepo(forge.NewSQLRepo[*AgentJob](db), cfg)
}

// newWithRepo creates a Module using an explicit repository. Used in tests.
func newWithRepo(repo forge.Repository[*AgentJob], cfg Config) *Module {
	m := &Module{cfg: cfg, repo: repo}
	m.mod = forge.NewModule((*AgentJob)(nil),
		forge.At("/agent-jobs"),
		forge.Repo(repo),
		forge.MCP(forge.MCPRead, forge.MCPWrite),
		forge.On(forge.AfterPublish, m.rebuildOnChange),
		forge.On(forge.AfterArchive, m.rebuildOnChange),
	)
	return m
}

// Register wires the module into app: registers the AgentJob content type
// (HTTP routes + MCP tools), subscribes to the signal bus for signal-triggered
// jobs, and starts the cron scheduler from currently published jobs.
//
// Call Register after all other content modules have been registered with app
// so that signal subscriptions are established before app.Run().
func (m *Module) Register(app *forge.App) {
	app.Content(m.mod)

	for _, sig := range []forge.Signal{
		forge.AfterCreate, forge.AfterUpdate,
		forge.AfterPublish, forge.AfterUnpublish,
		forge.AfterArchive, forge.AfterSchedule, forge.AfterDelete,
	} {
		s := sig
		app.OnSignal(s, func(ctx context.Context, ev forge.SignalEvent) error {
			return m.handleSignal(ctx, s, ev)
		})
	}

	if err := m.rebuildScheduler(context.Background()); err != nil {
		slog.Error("forge-agent: scheduler init failed", "error", err)
	}
}

// Stop gracefully shuts down the cron scheduler. Call this in the
// application's shutdown handler.
func (m *Module) Stop() {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.sched != nil {
		m.sched.Stop()
		m.sched = nil
	}
}

// rebuildOnChange is registered as an AfterPublish and AfterArchive handler
// on the AgentJob module. It rebuilds the cron scheduler whenever an AgentJob
// transitions in or out of Published status.
func (m *Module) rebuildOnChange(ctx forge.Context, _ *AgentJob) error {
	return m.rebuildScheduler(ctx)
}

// rebuildScheduler queries all published AgentJobs with cron triggers, builds
// a new [agent.Scheduler], and replaces the running one atomically.
func (m *Module) rebuildScheduler(ctx context.Context) error {
	jobs, err := m.repo.FindAll(ctx, forge.ListOptions{
		Status: []forge.Status{forge.Published},
	})
	if err != nil {
		return fmt.Errorf("forge-agent: load jobs: %w", err)
	}

	var agentJobs []agent.Job
	for _, j := range jobs {
		if !j.isCronTrigger() {
			continue
		}
		agentJobs = append(agentJobs, agent.Job{
			Schedule: j.Trigger,
			Task:     buildTask(j),
			Config:   m.agentConfig(j),
		})
	}

	sched, err := agent.NewScheduler(agentJobs)
	if err != nil {
		return fmt.Errorf("forge-agent: build scheduler: %w", err)
	}

	m.mu.Lock()
	if m.sched != nil {
		m.sched.Stop()
	}
	m.sched = sched
	m.mu.Unlock()

	sched.Start()
	return nil
}

// handleSignal is called by app.OnSignal for every content lifecycle signal.
// It fires published AgentJobs whose Trigger matches the signal and whose
// ContentTypeFilter matches the event's content type.
func (m *Module) handleSignal(ctx context.Context, sig forge.Signal, ev forge.SignalEvent) error {
	slog.Info("forge-agent: handleSignal called",
		"signal", string(sig), "type", ev.Type, "slug", ev.Slug)

	// Guard: never trigger jobs in response to AgentJob lifecycle events.
	// This prevents a job with an empty ContentTypeFilter from activating
	// other jobs whenever an AgentJob is published or archived.
	if ev.Type == "AgentJob" {
		return nil
	}

	jobs, err := m.repo.FindAll(ctx, forge.ListOptions{
		Status: []forge.Status{forge.Published},
	})
	if err != nil {
		return fmt.Errorf("forge-agent: load jobs for signal %s: %w", sig, err)
	}
	slog.Info("forge-agent: signal jobs loaded", "count", len(jobs))

	// Detach from the request context so the agent run is not cancelled when
	// the triggering HTTP request finishes.
	runCtx := context.WithoutCancel(ctx)
	for _, j := range jobs {
		matched := matchesSignal(j, sig, ev)
		slog.Info("forge-agent: signal job evaluated", "job", j.Name, "matched", matched)
		if !matched {
			continue
		}
		job := j
		task := buildSignalTask(job, ev)
		slog.Info("forge-agent: starting agent run",
			"job", job.Name, "signal", string(sig), "type", ev.Type, "slug", ev.Slug)
		go func() {
			_, runErr := agent.New(m.agentConfig(job)).Run(runCtx, task)
			if runErr != nil {
				slog.Error("forge-agent: signal job failed",
					"job", job.Name,
					"signal", sig,
					"content_type", ev.Type,
					"slug", ev.Slug,
					"error", runErr,
				)
			} else {
				slog.Info("forge-agent: agent run complete", "job", job.Name)
			}
		}()
	}
	return nil
}

// matchesSignal reports whether j should fire in response to sig/ev.
// Pure function — used in handleSignal and directly testable.
func matchesSignal(j *AgentJob, sig forge.Signal, ev forge.SignalEvent) bool {
	if j.isCronTrigger() {
		return false
	}
	if string(sig) != j.Trigger {
		return false
	}
	if j.ContentTypeFilter != "" && j.ContentTypeFilter != ev.Type {
		return false
	}
	return true
}

func (m *Module) agentConfig(j *AgentJob) agent.Config {
	return agent.Config{
		MCPURL:         m.cfg.MCPURL,
		MCPToken:       m.cfg.MCPToken,
		StreamableHTTP: m.cfg.StreamableHTTP,
		SystemPrompt:   j.SystemPrompt,
		Model:          j.Model,
		MaxTurns:       j.MaxTurns,
	}
}

// buildTask returns the task prompt for a cron-triggered agent run.
// If WebhookURL is set, instructs the agent to POST output there via http_post.
func buildTask(j *AgentJob) string {
	task := "Execute your instructions as defined in the system prompt."
	if j.WebhookURL != "" {
		task += " Use the http_post tool to send your output to: " + j.WebhookURL
	}
	return task
}

// buildSignalTask returns the task prompt for a signal-triggered agent run,
// enriched with the full SignalEvent serialized as JSON.
func buildSignalTask(j *AgentJob, ev forge.SignalEvent) string {
	evJSON, _ := json.Marshal(ev)
	task := fmt.Sprintf(
		"A new %s lifecycle event occurred: %s\nExecute your instructions as defined in the system prompt.",
		ev.Type, string(evJSON),
	)
	if j.WebhookURL != "" {
		task += " Use the http_post tool to send your output to: " + j.WebhookURL
	}
	return task
}
