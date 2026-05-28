// AGPL-3.0-or-later

package forgeagent

import (
	"context"
	"strings"
	"testing"
	"time"

	"smeldr.dev/core"
)

// — isCronTrigger ————————————————————————————————————————————————————————————

func TestIsCronTrigger(t *testing.T) {
	cases := []struct {
		trigger string
		want    bool
	}{
		{"45 13 * * *", true},
		{"0 6 * * 1-5", true},
		{"*/5 * * * *", true},
		{"after_publish", false},
		{"after_create", false},
		{"after_archive", false},
		{"", false},
	}
	for _, tc := range cases {
		j := &AgentJob{Trigger: tc.trigger}
		if got := j.isCronTrigger(); got != tc.want {
			t.Errorf("isCronTrigger(%q) = %v, want %v", tc.trigger, got, tc.want)
		}
	}
}

// — matchesSignal ————————————————————————————————————————————————————————————

func TestMatchesSignal(t *testing.T) {
	ev := func(typ string) smeldr.SignalEvent {
		return smeldr.SignalEvent{Type: typ, Slug: "test-slug"}
	}

	cases := []struct {
		name    string
		trigger string
		filter  string
		sig     smeldr.Signal
		evType  string
		want    bool
	}{
		{
			name:    "cron trigger never matches signal",
			trigger: "45 13 * * *", sig: smeldr.AfterPublish, evType: "Post",
			want: false,
		},
		{
			name:    "signal match no filter",
			trigger: "after_publish", sig: smeldr.AfterPublish, evType: "Post",
			want: true,
		},
		{
			name:    "signal match with matching filter",
			trigger: "after_publish", filter: "Post", sig: smeldr.AfterPublish, evType: "Post",
			want: true,
		},
		{
			name:    "signal match with non-matching filter",
			trigger: "after_publish", filter: "Story", sig: smeldr.AfterPublish, evType: "Post",
			want: false,
		},
		{
			name:    "wrong signal",
			trigger: "after_create", sig: smeldr.AfterPublish, evType: "Post",
			want: false,
		},
		{
			name:    "AgentJob type with empty filter",
			trigger: "after_publish", sig: smeldr.AfterPublish, evType: "AgentJob",
			want: true, // matchesSignal itself does not guard — handleSignal does
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			j := &AgentJob{Trigger: tc.trigger, ContentTypeFilter: tc.filter}
			got := matchesSignal(j, tc.sig, ev(tc.evType))
			if got != tc.want {
				t.Errorf("matchesSignal = %v, want %v", got, tc.want)
			}
		})
	}
}

// — handleSignal guard ————————————————————————————————————————————————————————

func TestHandleSignalSkipsAgentJobEvents(t *testing.T) {
	repo := smeldr.NewMemoryRepo[*AgentJob]()
	ctx := context.Background()

	// Seed an active job that would otherwise match after_publish / all types.
	j := &AgentJob{
		Node:         smeldr.Node{ID: smeldr.NewID(), Slug: "catch-all", Status: smeldr.Published},
		Name:         "catch-all",
		Trigger:      "after_publish",
		SystemPrompt: "test",
	}
	if err := repo.Save(ctx, j); err != nil {
		t.Fatal(err)
	}

	m := newWithRepo(repo, Config{})
	ev := smeldr.SignalEvent{Type: "AgentJob", Slug: "some-job"}

	// Should return nil without attempting to fire any agent run.
	if err := m.handleSignal(ctx, smeldr.AfterPublish, ev); err != nil {
		t.Errorf("handleSignal returned error: %v", err)
	}
}

// — rebuildScheduler ——————————————————————————————————————————————————————————

func TestRebuildSchedulerEmpty(t *testing.T) {
	repo := smeldr.NewMemoryRepo[*AgentJob]()
	m := newWithRepo(repo, Config{})
	if err := m.rebuildScheduler(context.Background()); err != nil {
		t.Fatalf("rebuildScheduler with no jobs: %v", err)
	}
	m.Stop()
}

func TestRebuildSchedulerCronJobs(t *testing.T) {
	repo := smeldr.NewMemoryRepo[*AgentJob]()
	ctx := context.Background()

	now := time.Now().UTC()
	for _, j := range []*AgentJob{
		{
			Node:         smeldr.Node{ID: smeldr.NewID(), Slug: "job-1", Status: smeldr.Published, CreatedAt: now, UpdatedAt: now},
			Name:         "job-1",
			Trigger:      "0 6 * * *",
			SystemPrompt: "run daily",
		},
		{
			Node:         smeldr.Node{ID: smeldr.NewID(), Slug: "job-2", Status: smeldr.Published, CreatedAt: now, UpdatedAt: now},
			Name:         "job-2",
			Trigger:      "after_publish", // signal trigger — should not enter scheduler
			SystemPrompt: "react to publish",
		},
		{
			Node:         smeldr.Node{ID: smeldr.NewID(), Slug: "job-3", Status: smeldr.Draft, CreatedAt: now, UpdatedAt: now},
			Name:         "job-3",
			Trigger:      "0 8 * * *", // draft — should be excluded
			SystemPrompt: "draft job",
		},
	} {
		if err := repo.Save(ctx, j); err != nil {
			t.Fatal(err)
		}
	}

	m := newWithRepo(repo, Config{})
	if err := m.rebuildScheduler(ctx); err != nil {
		t.Fatalf("rebuildScheduler: %v", err)
	}
	m.Stop()
}

// — AgentJob ContentTitle ————————————————————————————————————————————————————

func TestAgentJobContentTitle(t *testing.T) {
	j := &AgentJob{Name: "devlog-social-drafts"}
	if got := j.ContentTitle(); got != "devlog-social-drafts" {
		t.Errorf("ContentTitle = %q, want %q", got, "devlog-social-drafts")
	}
}

// — buildTask ————————————————————————————————————————————————————————————————

func TestBuildTask(t *testing.T) {
	base := "Execute your instructions as defined in the system prompt."

	t.Run("no webhook", func(t *testing.T) {
		j := &AgentJob{}
		if got := buildTask(j); got != base {
			t.Errorf("buildTask = %q, want %q", got, base)
		}
	})

	t.Run("with webhook", func(t *testing.T) {
		j := &AgentJob{WebhookURL: "https://example.com/hook"}
		want := base + " Use the http_post tool to send your output to: https://example.com/hook"
		if got := buildTask(j); got != want {
			t.Errorf("buildTask = %q, want %q", got, want)
		}
	})
}

// — buildSignalTask ——————————————————————————————————————————————————————————

func TestBuildSignalTask(t *testing.T) {
	t.Run("no webhook", func(t *testing.T) {
		ev := smeldr.SignalEvent{Type: "Post", Slug: "hello-world", Title: "Hello World", URL: "https://example.com/posts/hello-world"}
		got := buildSignalTask(&AgentJob{}, ev)
		if !strings.Contains(got, `"hello-world"`) {
			t.Errorf("expected slug in task, got: %s", got)
		}
		if !strings.Contains(got, "A new Post lifecycle event occurred") {
			t.Errorf("expected type prefix in task, got: %s", got)
		}
		if !strings.Contains(got, "Execute your instructions") {
			t.Errorf("expected closing instruction in task, got: %s", got)
		}
	})

	t.Run("with webhook", func(t *testing.T) {
		ev := smeldr.SignalEvent{Type: "Post", Slug: "hello-world"}
		got := buildSignalTask(&AgentJob{WebhookURL: "https://example.com/hook"}, ev)
		if !strings.Contains(got, "http_post") {
			t.Errorf("expected http_post instruction, got: %s", got)
		}
		if !strings.Contains(got, "https://example.com/hook") {
			t.Errorf("expected webhook URL in task, got: %s", got)
		}
	})

	t.Run("json contains all event fields", func(t *testing.T) {
		ev := smeldr.SignalEvent{Type: "Story", Slug: "my-story", Title: "My Story", URL: "https://example.com/stories/my-story"}
		got := buildSignalTask(&AgentJob{}, ev)
		for _, want := range []string{`"my-story"`, `"My Story"`, `"https://example.com/stories/my-story"`} {
			if !strings.Contains(got, want) {
				t.Errorf("expected %s in task, got: %s", want, got)
			}
		}
	})
}
