package schedule

import (
	"context"
	"os"
	"slices"
	"strings"
	"time"

	"pkg/ark"
)

// OverdueGrace is how late a schedule may be before Health reports it overdue.
const OverdueGrace = time.Hour

// ScheduleHealth is the health of one enabled schedule.
type ScheduleHealth struct {
	Name   string
	OK     bool
	Detail string
}

// Health checks each enabled schedule of a known type, in name order: a held lock is OK "running"; a run left
// "running" with the lock free or a "failed" run is not OK; otherwise (including "skipped") it is overdue when now is past
// Next(base)+OverdueGrace, where base is the latest of the last start, since (the scheduler's start) and the
// compose file's modification time, so a schedule enabled or edited after a long pause is not overdue.
func (s *Service) Health(ctx context.Context, compose ark.ComposeService, composeFile string, now, since time.Time) []ScheduleHealth {
	file, err := s.LoadScheduleFile(ctx, compose, composeFile)
	if err != nil {
		return []ScheduleHealth{{Name: "schedules", OK: false, Detail: err.Error()}}
	}
	base := since
	if info, err := os.Stat(composeFile); err == nil && info.ModTime().After(base) {
		base = info.ModTime()
	}
	sorted := slices.SortedFunc(slices.Values(file.Schedules), func(a, b BackupSchedule) int {
		return strings.Compare(a.Name, b.Name)
	})
	result := []ScheduleHealth{}
	for _, sched := range sorted {
		if sched.Enabled && KnownType(sched.Type) {
			result = append(result, s.scheduleHealth(sched, now, base))
		}
	}
	return result
}

func (s *Service) scheduleHealth(sched BackupSchedule, now, base time.Time) ScheduleHealth {
	fail := func(detail string) ScheduleHealth { return ScheduleHealth{Name: sched.Name, Detail: detail} }
	if s.LockHeld(sched.Name) {
		return ScheduleHealth{Name: sched.Name, OK: true, Detail: "running"}
	}
	state, _ := s.LoadRunState(sched.Name)
	switch state.LastStatus {
	case "running":
		return fail("last run did not finish (started " + state.LastStartedAt + ")")
	case "failed":
		return fail("last run failed: " + state.LastError)
	}
	cronSched, err := ParseCron(sched.Cron)
	if err != nil {
		return fail("invalid cron")
	}
	// An unreadable last start leaves base as it is.
	if started, err := time.Parse(time.RFC3339, state.LastStartedAt); err == nil && started.After(base) {
		base = started
	}
	next := cronSched.Next(base.In(time.Local))
	if now.After(next.Add(OverdueGrace)) {
		return fail("overdue since " + next.Format(time.RFC3339))
	}
	return ScheduleHealth{Name: sched.Name, OK: true}
}
