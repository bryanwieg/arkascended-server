package compose

// Tests: start --instance all also ups the scheduler service when the project has one; a failed scheduler up only warns

import (
	"ark-cli/internal/core"
	"context"
	"errors"
	"fmt"
	"reflect"
	"strings"
	"testing"

	"github.com/compose-spec/compose-go/v2/types"

	"pkg/ark"
	"pkg/ark/arktest"
)

// runStartAll runs `start instance --instance all --no-wait` against a mock compose and returns the
// services of each Up call and the warnings printed.
func runStartAll(t *testing.T, withScheduler bool, schedulerUpErr error) ([][]string, string) {
	t.Helper()
	t.Setenv("ARK_BACKUP_DIR", t.TempDir())
	services := types.Services{"asa1": {Name: "asa1", Labels: types.Labels{ark.PortASALabel: "27015"}, Image: "ark-server:abc123"}}
	if withScheduler {
		services[ark.SchedulerService] = types.ServiceConfig{Name: ark.SchedulerService, Labels: types.Labels{ark.RoleLabel: ark.SchedulerRole}}
	}
	var ups [][]string
	var warnings strings.Builder
	origWarnf := core.Warnf
	core.Warnf = func(format string, args ...any) { fmt.Fprintf(&warnings, format, args...) }
	t.Cleanup(func() { core.Warnf = origWarnf })
	withPatchedAppDeps(t, func(d *core.AppDeps) {
		d.Engine = &arktest.MockEngine{ImageExistsFn: func(context.Context, string) (bool, error) { return true, nil }}
		d.DiskUsageForPath = func(string) (int64, int64, error) { return 1 << 40, 1 << 40, nil }
		d.ComposeService = &arktest.MockCompose{
			LoadProjectFn: func(context.Context, string) (*ark.ComposeProject, error) { return fakeProject(services), nil },
			UpFn: func(_ context.Context, _ string, svcs []string, _ ark.UpOptions) error {
				ups = append(ups, svcs)
				if svcs[0] == ark.SchedulerService {
					return schedulerUpErr
				}
				return nil
			},
		}
	}, func() {
		if err := RunStart(core.CurrentAppDeps(), []string{"--instance", "all", "--no-wait"}); err != nil {
			t.Fatalf("RunStart: %v", err)
		}
	})
	return ups, warnings.String()
}

func TestRunStartAllUpsSchedulerWhenPresent(t *testing.T) {
	ups, _ := runStartAll(t, true, nil)
	if want := [][]string{{"asa1"}, {ark.SchedulerService}}; !reflect.DeepEqual(ups, want) {
		t.Fatalf("Up calls = %v, want %v", ups, want)
	}
}

func TestRunStartAllWithoutSchedulerUpsOnlyInstances(t *testing.T) {
	ups, _ := runStartAll(t, false, nil)
	if want := [][]string{{"asa1"}}; !reflect.DeepEqual(ups, want) {
		t.Fatalf("Up calls = %v, want %v", ups, want)
	}
}

func TestRunStartAllSchedulerUpFailureOnlyWarns(t *testing.T) {
	_, warnings := runStartAll(t, true, errors.New("image not found"))
	if !strings.Contains(warnings, "Warning: the scheduler did not start: image not found. Run 'ark-cli enable scheduler'.") {
		t.Fatalf("warnings = %q", warnings)
	}
}
