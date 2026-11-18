package update

import (
	"ark-cli/internal/compose"
	"ark-cli/internal/core"
	"context"
	"errors"
	"fmt"
	"os"
	"sort"
	"strings"
	"time"

	"github.com/compose-spec/compose-go/v2/types"

	"pkg/ark/backup"
	"pkg/ark/composeutil"
	"pkg/ark/runtime"
	"pkg/ark/update"
)

func resolveUpdateTargetServices(project *types.Project, target string) ([]string, error) {
	if len(project.Services) == 0 {
		return nil, errors.New("Error: services section not found in compose file.")
	}
	if target == "all" {
		names := make([]string, 0, len(project.Services))
		for name := range project.Services {
			names = append(names, name)
		}
		sort.Strings(names)
		return names, nil
	}
	if _, ok := project.Services[target]; !ok {
		return nil, fmt.Errorf("Error: service not found in compose file: %s", target)
	}
	return []string{target}, nil
}

// plannedUpdate switches an instance from one image to another.
type plannedUpdate struct{ Instance, From, To string }

// planAndApplyImageUpdates checks each service against Steam and, unless
// req.CheckOnly, builds the new image once for every service that needs it.
func planAndApplyImageUpdates(
	project *types.Project,
	services []string,
	composeFile string,
	req UpdateCommandRequest,
) ([]update.PlanRow, []plannedUpdate, error) {
	rows := make([]update.PlanRow, 0, len(services))
	pending := []plannedUpdate{} // To is filled in once the image is built
	localChecks := map[string]update.LocalCheckResult{}

	d := core.CurrentAppDeps()
	ctx := core.CurrentAppDeps().Context()

	for _, serviceName := range services {
		svc := project.Services[serviceName]
		row, err := planImageUpdateForService(ctx, d, serviceName, svc, localChecks)
		if err != nil {
			return nil, nil, err
		}
		rows = append(rows, row)
		if !req.CheckOnly && row.Action == update.ActionUpdate {
			pending = append(pending, plannedUpdate{Instance: serviceName, From: svc.Image})
		}
	}
	if len(pending) == 0 {
		return rows, nil, nil
	}

	stateDir := update.StateDir(composeFile)
	built, err := d.UpdateService.BuildLocalImage(ctx, stateDir)
	if err != nil {
		for _, u := range pending {
			recordBuildFailure(stateDir, u, err)
		}
		return nil, nil, err
	}
	updates := make([]plannedUpdate, len(pending))
	for i, u := range pending {
		updates[i] = plannedUpdate{Instance: u.Instance, From: u.From, To: built.Image}
	}
	sort.Slice(updates, func(i, j int) bool { return updates[i].Instance < updates[j].Instance })
	return rows, updates, nil
}

// recordBuildFailure records a failed image build as the instance's last attempt;
// a state error only warns, the build error is what the command reports.
func recordBuildFailure(stateDir string, u plannedUpdate, buildErr error) {
	attempt := update.UpdateAttempt{
		Time: time.Now().UTC(), From: u.From, Result: update.ResultBuildFailed, Message: buildErr.Error(),
	}
	st, err := update.LoadState(stateDir, u.Instance)
	if err == nil {
		st.LastAttempt = &attempt
		err = update.SaveState(stateDir, u.Instance, st)
	}
	if err != nil {
		core.Warnf("Warning: could not record the failed build for %s: %v\n", u.Instance, err)
	}
	alertFailure(u.Instance, attempt)
}

// alertFailure posts a failed update to ARK_UPDATE_WEBHOOK_URL when it is set.
// A webhook problem only warns: it never changes the command's result.
func alertFailure(instance string, attempt update.UpdateAttempt) {
	url, err := update.WebhookURL()
	if err != nil {
		core.Warnf("Warning: %v\n", err)
		return
	}
	if url == "" {
		return
	}
	host, _ := os.Hostname()
	msg := update.FormatAttemptMessage(host, instance, attempt)
	if err := update.NotifyUpdate(core.CurrentAppDeps().Context(), url, msg); err != nil {
		core.Warnf("Warning: update alert webhook failed: %v\n", err)
	}
}

func planImageUpdateForService(
	ctx context.Context,
	d *core.AppDeps,
	serviceName string,
	service types.ServiceConfig,
	localChecks map[string]update.LocalCheckResult,
) (update.PlanRow, error) {
	image := strings.TrimSpace(service.Image)
	if image == "" {
		return update.PlanRow{
			Instance:     serviceName,
			CurrentImage: "-",
			Action:       update.ActionWarn,
			Message:      "service has no image configured",
		}, nil
	}
	if repo, _, _ := strings.Cut(image, ":"); repo != update.DefaultLocalImageName {
		return update.PlanRow{
			Instance:     serviceName,
			CurrentImage: image,
			Action:       update.ActionWarn,
			Message:      "only locally built ark-server images are supported",
		}, nil
	}
	check, ok := localChecks[image]
	if !ok {
		resolved, err := d.UpdateService.CheckLocalUpdate(ctx, image)
		if err != nil {
			return update.PlanRow{}, err
		}
		check = resolved
		localChecks[image] = check
	}
	return update.PlanLocalUpdate(serviceName, image, check), nil
}

func printImageUpdateTable(rows []update.PlanRow) {
	core.Infof("instance | current | target | action | note\n")
	core.Infof("---------|---------|--------|--------|-----\n")
	for _, row := range rows {
		current := runtime.FallbackDash(row.CurrentVersion)
		if current == "-" {
			current = runtime.FallbackDash(row.CurrentImage)
		}
		target := runtime.FallbackDash(row.TargetVersion)
		if target == "-" {
			target = runtime.FallbackDash(row.TargetImage)
		}
		core.Infof("%s | %s | %s | %s | %s\n", row.Instance, current, target, row.Action, runtime.FallbackDash(row.Message))
	}
}

// rolloutUpdatedInstances switches each instance to its new image in order, rolling
// back on failure (update.Rollout). The first rollback stops the run: a build that
// broke one instance would break the rest.
func rolloutUpdatedInstances(composeFile string, updates []plannedUpdate) error {
	if len(updates) == 0 {
		core.Infof("No instances require restart.\n")
		return nil
	}
	d := core.CurrentAppDeps()
	steps := newRolloutSteps(d, composeFile)
	stateDir := update.StateDir(composeFile)
	for i, u := range updates {
		core.Infof("Rolling update: %s (%s -> %s)\n", u.Instance, u.From, u.To)
		attempt, err := update.Rollout(d.Context(), update.RolloutRequest{
			StateDir: stateDir, Instance: u.Instance, From: u.From, To: u.To,
		}, steps)
		if attempt.Result == update.ResultRolledBack || attempt.Result == update.ResultRollbackFailed {
			alertFailure(u.Instance, attempt)
		}
		if errors.Is(err, update.ErrRolledBack) {
			core.Warnf("Update of %s to %s failed; rolled back to %s: %s\n", u.Instance, u.To, u.From, attempt.Message)
			if rest := updates[i+1:]; len(rest) > 0 {
				names := make([]string, len(rest))
				for j, r := range rest {
					names[j] = r.Instance
				}
				core.Warnf("Skipped remaining instances: %s\n", strings.Join(names, ", "))
			}
			return fmt.Errorf("Error: update of %s rolled back", u.Instance)
		}
		if err != nil {
			return err
		}
		core.Infof("Updated %s: %s -> %s\n", u.Instance, u.From, u.To)
	}
	core.Infof("Rolling update complete for %d instance(s).\n", len(updates))
	return nil
}

// newRolloutSteps returns the real rollout operations; tests replace it.
var newRolloutSteps = func(d *core.AppDeps, composeFile string) update.RolloutSteps {
	backupRoot := core.BackupRootDirForComposeFile(composeFile)
	return update.RolloutSteps{
		Stop: func(_ context.Context, instance string) error {
			return compose.DoRestartStopInstance(d, runtime.StopCommandRequest{Instance: instance, ComposeFileFlag: composeFile, Timeout: -1})
		},
		Backup: func(ctx context.Context, instance string) (string, error) {
			// Same defaults as `create backup`.
			result, err := d.BackupService.CreateForTarget(ctx, backup.CreateRequest{
				ComposeFile:       composeFile,
				Service:           instance,
				BackupRoot:        backupRoot,
				NameOverride:      "pre-update",
				Format:            "tar.gz",
				Timestamp:         time.Now().UTC().Format("20060102-150405"),
				PID:               os.Getpid(),
				LockTimeout:       2 * time.Minute,
				BackupWaitTimeout: 10 * time.Minute,
				Retention:         backup.NewRetentionPolicy(20, 7, 5),
			}, instance)
			if err != nil {
				return "", err
			}
			return result.ArchivePath, nil
		},
		SetImage: func(ctx context.Context, instance, image string) error {
			project, err := d.ComposeService.LoadProject(ctx, composeFile)
			if err != nil {
				return err
			}
			if err := composeutil.MutateService(project.Project, instance, func(s *types.ServiceConfig) { s.Image = image }); err != nil {
				return err
			}
			return d.ComposeService.SaveProject(ctx, composeFile, project)
		},
		StartAndWait: func(_ context.Context, instance string) error {
			return compose.DoRestartStartInstance(d, runtime.StartCommandRequest{
				Instance:        instance,
				ComposeFileFlag: composeFile,
				WaitTimeout:     compose.DefaultStartReadyWaitTimeout,
			})
		},
		Restore: func(ctx context.Context, instance, archive string) error {
			_, err := d.BackupService.Restore(ctx, backup.RestoreRequest{
				ComposeFile: composeFile, Service: instance, ArchivePath: archive, BackupRoot: backupRoot,
			})
			return err
		},
		Now: func() time.Time { return time.Now().UTC() },
	}
}

func uniqueSortedStrings(items []string) []string {
	set := map[string]struct{}{}
	for _, item := range items {
		item = strings.TrimSpace(item)
		if item == "" {
			continue
		}
		set[item] = struct{}{}
	}
	out := make([]string, 0, len(set))
	for item := range set {
		out = append(out, item)
	}
	sort.Strings(out)
	return out
}
