package agent

import (
	"testing"
)

func TestNewScheduler_valid(t *testing.T) {
	jobs := []Job{
		{
			Schedule: "0 6 * * *",
			Timezone: "Europe/Copenhagen",
			Task:     "test task",
			Config:   Config{},
		},
	}
	s, err := NewScheduler(jobs)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if s == nil {
		t.Fatal("expected non-nil Scheduler")
	}
}

func TestNewScheduler_invalidTimezone(t *testing.T) {
	jobs := []Job{
		{
			Schedule: "0 6 * * *",
			Timezone: "Not/ATimezone",
			Task:     "test task",
		},
	}
	_, err := NewScheduler(jobs)
	if err == nil {
		t.Fatal("expected error for invalid timezone, got nil")
	}
}

func TestNewScheduler_emptyTimezoneDefaultsToUTC(t *testing.T) {
	jobs := []Job{
		{Schedule: "0 6 * * *", Task: "task"},
	}
	s, err := NewScheduler(jobs)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if s == nil {
		t.Fatal("expected non-nil Scheduler")
	}
}

func TestScheduler_jobFields(t *testing.T) {
	j := Job{
		Schedule: "*/5 * * * *",
		Timezone: "America/New_York",
		Task:     "do something",
		Config:   Config{Model: "claude-sonnet-4-6", MaxTurns: 5},
	}
	if j.Schedule != "*/5 * * * *" {
		t.Errorf("Schedule: got %q", j.Schedule)
	}
	if j.Timezone != "America/New_York" {
		t.Errorf("Timezone: got %q", j.Timezone)
	}
	if j.Task != "do something" {
		t.Errorf("Task: got %q", j.Task)
	}
	if j.Config.MaxTurns != 5 {
		t.Errorf("Config.MaxTurns: got %d", j.Config.MaxTurns)
	}
}

func TestScheduler_startStop(t *testing.T) {
	jobs := []Job{
		{Schedule: "0 0 * * *", Task: "daily task"},
	}
	s, err := NewScheduler(jobs)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	s.Start()
	s.Stop() // must not panic
}

func TestNewScheduler_multipleTimezones(t *testing.T) {
	jobs := []Job{
		{Schedule: "0 6 * * *", Timezone: "Europe/Copenhagen", Task: "task A"},
		{Schedule: "0 7 * * *", Timezone: "America/New_York", Task: "task B"},
		{Schedule: "0 8 * * *", Timezone: "Europe/Copenhagen", Task: "task C"},
	}
	s, err := NewScheduler(jobs)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// Two unique timezones → two internal schedulers.
	if len(s.schedulers) != 2 {
		t.Errorf("expected 2 internal schedulers, got %d", len(s.schedulers))
	}
}
