package scheduler

import (
	"errors"
	"fmt"
	"io/fs"
	"maps"
	"path/filepath"
	"regexp"
	"slices"
	"time"

	"ark-cli/internal/compose"
	"ark-cli/internal/core"

	"pkg/ark"
	"pkg/ark/health"
	"pkg/ark/schedule"
)

// RunCheckScheduler prints the scheduler's health report; an error (exit 1) when it is missing, stale or
// unhealthy. The scheduler service uses it as its Docker healthcheck.
func RunCheckScheduler(d *core.AppDeps, args []string) error {
	flags := core.NewCommandFlagSet("check scheduler", []string{"check scheduler [--json]"}, "Show scheduler health (exit 1 when unhealthy, stale or missing).")
	composeFileFlag := flags.String("compose-file", "", "Path to docker-compose.yaml (default: ./docker-compose.yaml or COMPOSE_FILE).")
	jsonFlag := flags.Bool("json", false, "Print the health report as JSON.")
	if err := core.ParseNoPositionalArgs(flags, args); err != nil {
		return err
	}
	composeFile := compose.ResolveComposeFile(*composeFileFlag)
	warnImageMismatch(d, composeFile)
	path := healthPath(composeFile)
	report, err := health.LoadReport(path)
	switch {
	case errors.Is(err, fs.ErrNotExist):
		return fmt.Errorf("Error: no scheduler health report at %s; is the scheduler running?", path)
	case err != nil:
		return fmt.Errorf("Error: unreadable scheduler health report: %w", err)
	case time.Since(report.Time) > health.ReportStaleAfter:
		return fmt.Errorf("Error: scheduler health report is stale (last update %s)", report.Time.Format(time.RFC3339))
	}
	if *jsonFlag {
		if err := core.PrintJSONOutput(report); err != nil { // a core.SchedulerHealthJSON (alias; a conversion trips unconvert)
			return err
		}
	} else {
		printReport(report)
	}
	if !report.Healthy {
		return errors.New("Error: scheduler reports unhealthy")
	}
	return nil
}

// printReport prints the report as a table of checks, then the compose error and recent restarts.
func printReport(r health.Report) {
	core.Infof("Health report of %s\n", r.Time.Format(time.RFC3339))
	rows := [][]string{{"INSTANCE", "STATE", "CHECK", "RESULT", "DETAIL"}}
	for _, name := range slices.Sorted(maps.Keys(r.Instances)) {
		inst := r.Instances[name]
		for _, check := range health.InstanceChecks {
			rows = append(rows, []string{name, inst.State, check, inst.Checks[check], inst.Details[check]})
		}
	}
	for _, name := range slices.Sorted(maps.Keys(r.Schedules)) {
		rows = append(rows, []string{"-", "-", name, r.Schedules[name], r.ScheduleDetails[name]})
	}
	core.PrintTable(rows)
	if r.Error != "" {
		core.Infof("Compose error: %s\n", r.Error)
	}
	for _, rec := range r.Restarts {
		failed := ""
		if rec.Error != "" {
			failed = ", failed: " + rec.Error
		}
		core.Infof("Restarted %s at %s (%s%s)\n", rec.Instance, rec.Time.Format(time.RFC3339), rec.Reason, failed)
	}
}

// warnImageMismatch warns when the compose file's scheduler image was not built from this binary. Silent when
// either cannot be read: the check is about health, the image tag is a hint. Silent too for an image enable
// scheduler did not name (such as the dev compose's ark-scheduler:dev): enable would replace that service.
// managedImageRE matches the image names schedule.ImageRef gives.
var managedImageRE = regexp.MustCompile(`^` + regexp.QuoteMeta(schedule.ImageRepo) + `:[0-9a-f]{12}$`)

func warnImageMismatch(d *core.AppDeps, composeFile string) {
	project, err := d.ComposeService.LoadProject(d.Context(), composeFile)
	if err != nil || !project.HasScheduler() {
		return
	}
	exe, err := executablePath()
	if err != nil {
		return
	}
	binary, err := filepath.EvalSymlinks(exe)
	if err != nil {
		return
	}
	ref, err := schedule.ImageRef(binary)
	if err != nil {
		return
	}
	image := project.Project.Services[ark.SchedulerService].Image
	if !managedImageRE.MatchString(image) {
		return
	}
	if image != ref {
		core.Warnf("Warning: the scheduler runs %s; run 'ark-cli enable scheduler' to update it to %s\n", image, ref)
	}
}
