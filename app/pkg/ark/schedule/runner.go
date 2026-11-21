package schedule

import (
	"context"
	"slices"
	"strings"
	"time"

	"github.com/robfig/cron/v3"
)

// Runner runs due backup schedules one at a time on a clock.
type Runner struct {
	Service *Service
	RunOne  func(ctx context.Context, s BackupSchedule) error // errors are logged, never stop the loop
	Now     func() time.Time                                  // default time.Now
	Logf    func(format string, args ...any)
}

// Run catches up once at start, then on each tick runs every enabled backup schedule due in (prev, now],
// in schedule-name order, one at a time, and returns when ctx is done (after the run in progress ends).
func (r *Runner) Run(ctx context.Context, tick <-chan time.Time) {
	start := r.now()
	r.CatchUp(ctx, start)
	prev := start
	for {
		select {
		case <-ctx.Done():
			return
		case <-tick:
			// The tick value can be stale (one stays buffered during a long run); the window uses the clock.
			prev = r.Step(ctx, prev, r.now())
		}
	}
}

// CatchUp runs each enabled backup schedule whose state has a LastStartedAt t with Next(t) <= now, or whose
// LastStatus is "running" while r.Service.LockHeld(name) is false (a run killed before it finished).
func (r *Runner) CatchUp(ctx context.Context, now time.Time) {
	r.runDue(ctx, func(s BackupSchedule, sched cron.Schedule) bool {
		state, ok := r.Service.LoadRunState(s.Name)
		if !ok || state.LastStartedAt == "" {
			return false
		}
		if state.LastStatus == "running" && !r.Service.LockHeld(s.Name) {
			return true
		}
		started, err := time.Parse(time.RFC3339, state.LastStartedAt)
		if err != nil {
			r.Logf("schedule %s: unreadable last start %q: %v", s.Name, state.LastStartedAt, err)
			return false
		}
		return !sched.Next(started.In(time.Local)).After(now.In(time.Local))
	})
}

// Step runs schedules with Next(prev) <= now and returns now (the next prev).
func (r *Runner) Step(ctx context.Context, prev, now time.Time) time.Time {
	r.runDue(ctx, func(_ BackupSchedule, sched cron.Schedule) bool {
		return !sched.Next(prev.In(time.Local)).After(now.In(time.Local))
	})
	return now
}

func (r *Runner) now() time.Time {
	if r.Now == nil {
		return time.Now()
	}
	return r.Now()
}

// runDue reloads the schedule file and runs, in name order, each enabled backup schedule that due accepts,
// stopping between runs once ctx is done.
func (r *Runner) runDue(ctx context.Context, due func(BackupSchedule, cron.Schedule) bool) {
	file, err := r.Service.LoadScheduleFile()
	if err != nil {
		r.Logf("schedule file unreadable: %v", err)
		return
	}
	sorted := slices.SortedFunc(slices.Values(file.Schedules), func(a, b BackupSchedule) int {
		return strings.Compare(a.Name, b.Name)
	})
	for _, s := range sorted {
		if ctx.Err() != nil {
			return
		}
		if !s.Enabled || s.Type != "backup" {
			continue
		}
		sched, err := ParseCron(s.Cron)
		if err != nil {
			r.Logf("schedule %s skipped: invalid cron %q: %v", s.Name, s.Cron, err)
			continue
		}
		if !due(s, sched) {
			continue
		}
		// A shutdown signal must not abort a backup midway.
		if err := r.RunOne(context.WithoutCancel(ctx), s); err != nil {
			r.Logf("schedule %s failed: %v", s.Name, err)
		}
	}
}
