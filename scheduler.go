package agent

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	gocron "github.com/go-co-op/gocron/v2"
)

// Job defines a single scheduled agent task.
type Job struct {
	// Schedule is a 5-field cron expression (e.g. "0 6 * * *").
	Schedule string
	// Timezone is the IANA timezone for the cron schedule (e.g. "Europe/Copenhagen").
	// Empty defaults to UTC.
	Timezone string
	// Task is the prompt passed to Agent.Run on each scheduled execution.
	Task string
	// Config is the agent configuration for this job.
	Config Config
}

// Scheduler wraps gocron and runs registered Jobs on their cron schedules.
// Jobs sharing a timezone share a gocron.Scheduler instance.
type Scheduler struct {
	schedulers []gocron.Scheduler
}

// NewScheduler creates a Scheduler from a slice of Jobs. Returns an error if
// any job has an invalid timezone or cron expression.
func NewScheduler(jobs []Job) (*Scheduler, error) {
	type tzGroup struct {
		loc  *time.Location
		jobs []Job
	}
	groups := map[string]*tzGroup{}
	order := []string{} // preserve insertion order for deterministic behaviour

	for _, j := range jobs {
		tz := j.Timezone
		if tz == "" {
			tz = "UTC"
		}
		if _, ok := groups[tz]; !ok {
			loc, err := time.LoadLocation(tz)
			if err != nil {
				return nil, fmt.Errorf("scheduler: invalid timezone %q: %w", tz, err)
			}
			groups[tz] = &tzGroup{loc: loc}
			order = append(order, tz)
		}
		groups[tz].jobs = append(groups[tz].jobs, j)
	}

	schedulers := make([]gocron.Scheduler, 0, len(groups))
	for _, tz := range order {
		g := groups[tz]
		s, err := gocron.NewScheduler(gocron.WithLocation(g.loc))
		if err != nil {
			return nil, fmt.Errorf("scheduler: %w", err)
		}
		for _, j := range g.jobs {
			job := j
			_, err := s.NewJob(
				gocron.CronJob(job.Schedule, false),
				gocron.NewTask(func(ctx context.Context) {
					result, runErr := New(job.Config).Run(ctx, job.Task)
					if runErr != nil {
						slog.Error("scheduler: job failed",
							"schedule", job.Schedule,
							"timezone", job.Timezone,
							"error", runErr,
						)
						return
					}
					slog.Info("scheduler: job done",
						"schedule", job.Schedule,
						"timezone", job.Timezone,
						"result", result,
					)
				}),
				gocron.WithSingletonMode(gocron.LimitModeReschedule),
			)
			if err != nil {
				return nil, fmt.Errorf("scheduler: register job %q: %w", job.Schedule, err)
			}
		}
		schedulers = append(schedulers, s)
	}

	return &Scheduler{schedulers: schedulers}, nil
}

// Start begins scheduling all jobs. Non-blocking.
func (s *Scheduler) Start() {
	for _, sc := range s.schedulers {
		sc.Start()
	}
}

// Stop gracefully shuts down all schedulers, waiting for in-flight jobs to complete.
func (s *Scheduler) Stop() {
	for _, sc := range s.schedulers {
		if err := sc.Shutdown(); err != nil {
			slog.Error("scheduler: shutdown error", "error", err)
		}
	}
}
