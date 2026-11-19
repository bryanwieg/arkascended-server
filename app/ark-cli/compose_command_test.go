package main

// Tests: compose start/stop/down commands: target selection, RCON env forwarding, verbose log tailing, local image builds

import (
	"ark-cli/internal/compose"
	"ark-cli/internal/core"
	"ark-cli/internal/remove"
	"context"
	"errors"
	"io"
	"sort"
	"strings"
	"sync"
	"testing"
	"time"

	"pkg/ark"
	"pkg/ark/arktest"
	"pkg/ark/health"

	"github.com/compose-spec/compose-go/v2/types"
)

const composeTestFile = "compose.yaml"

func composeTestServices(services []string) types.Services {
	svcMap := types.Services{}
	for _, name := range services {
		svcMap[name] = types.ServiceConfig{Name: name, Image: "ark-server:200"}
	}
	return svcMap
}

func withComposeLifecycleDeps(t *testing.T, services []string, patch func(*core.AppDeps), fn func()) {
	t.Helper()
	withPatchedAppDeps(t, func(d *core.AppDeps) {
		d.ComposeService = &arktest.MockCompose{
			LoadProjectFn: func(_ context.Context, _ string) (*ark.ComposeProject, error) {
				return fakeProject(composeTestServices(services)), nil
			},
		}
		d.Engine = &arktest.MockEngine{ // the pinned ark-server:200 is built already
			ImageExistsFn: func(context.Context, string) (bool, error) { return true, nil },
		}
		if patch != nil {
			patch(d)
		}
	}, fn)
}

func TestRunStartRunsComposeUpForTargets(t *testing.T) {
	tests := []struct {
		name        string
		args        []string
		services    []string
		wantTargets []string
		wantWaitFor []string
	}{
		{name: "all services", args: []string{"--instance", "all"}, services: []string{"a", "b"}, wantTargets: []string{"a", "b"}, wantWaitFor: []string{"a", "b"}},
		{name: "single service", args: []string{"--instance", "a"}, services: []string{"a"}, wantTargets: []string{"a"}, wantWaitFor: []string{"a"}},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			setEnv(t, "COMPOSE_FILE", composeTestFile)

			var upServices []string
			var upPullPolicy string
			waited := []string{}
			var waitedMu sync.Mutex // start waits for instances concurrently
			withComposeLifecycleDeps(t, tc.services, func(d *core.AppDeps) {
				loadProject := d.ComposeService.(*arktest.MockCompose).LoadProjectFn
				d.ComposeService = &arktest.MockCompose{
					LoadProjectFn: loadProject,
					UpFn: func(_ context.Context, _ string, services []string, opts ark.UpOptions) error {
						upServices = services
						upPullPolicy = opts.PullPolicy
						return nil
					},
				}
				d.RunningServiceSnapshot = func(_ string) (map[string]struct{}, error) {
					snapshot := map[string]struct{}{}
					for _, service := range tc.wantWaitFor {
						snapshot[service] = struct{}{}
					}
					return snapshot, nil
				}
				d.HealthChecker = &mockHealthChecker{
					waitReadyFn: func(_ context.Context, _, service string, _ health.WaitOptions) error {
						waitedMu.Lock()
						defer waitedMu.Unlock()
						waited = append(waited, service)
						return nil
					},
				}
			}, func() {
				if err := compose.RunStart(core.CurrentAppDeps(), tc.args); err != nil {
					t.Fatalf("runStart: %v", err)
				}
			})

			sortedUp := append([]string(nil), upServices...)
			sortedWant := append([]string(nil), tc.wantTargets...)
			sort.Strings(sortedUp)
			sort.Strings(sortedWant)
			if len(sortedUp) != len(sortedWant) {
				t.Fatalf("unexpected Up services: got %v want %v", upServices, tc.wantTargets)
			}
			for i := range sortedUp {
				if sortedUp[i] != sortedWant[i] {
					t.Fatalf("unexpected Up services: got %v want %v", upServices, tc.wantTargets)
				}
			}
			if upPullPolicy != "never" {
				t.Fatalf("unexpected pull policy: %s", upPullPolicy)
			}

			sortedWaited := append([]string(nil), waited...)
			sortedWantWait := append([]string(nil), tc.wantWaitFor...)
			sort.Strings(sortedWaited)
			sort.Strings(sortedWantWait)
			if len(sortedWaited) != len(sortedWantWait) {
				t.Fatalf("unexpected readiness waits: got %v want %v", waited, tc.wantWaitFor)
			}
			for i := range sortedWaited {
				if sortedWaited[i] != sortedWantWait[i] {
					t.Fatalf("unexpected readiness waits: got %v want %v", waited, tc.wantWaitFor)
				}
			}
		})
	}
}

// TestRunStopUsesComposeStop verifies that graceful stop uses the Stop interface with a timeout.
func TestRunStopUsesComposeStop(t *testing.T) {
	var gotServices []string
	var gotTimeout *time.Duration
	withComposeLifecycleDeps(t, []string{"a"}, func(d *core.AppDeps) {
		loadProject := d.ComposeService.(*arktest.MockCompose).LoadProjectFn
		d.ComposeService = &arktest.MockCompose{
			LoadProjectFn: loadProject,
			StopFn: func(_ context.Context, _ string, services []string, timeout *time.Duration) error {
				gotServices = services
				gotTimeout = timeout
				return nil
			},
		}
	}, func() {
		if err := compose.RunStop(core.CurrentAppDeps(), []string{"--instance", "a"}); err != nil {
			t.Fatalf("runStop: %v", err)
		}
	})

	if len(gotServices) != 1 || gotServices[0] != "a" {
		t.Fatalf("expected Stop called for service 'a', got %v", gotServices)
	}
	if gotTimeout != nil {
		t.Fatalf("expected nil timeout (compose stop_grace_period), got %v", *gotTimeout)
	}
}

// TestRunStartBuildsWhenMissing checks start builds the image when it is missing.
func TestRunStartBuildsWhenMissing(t *testing.T) {
	// An absolute path keeps the build's .ark-manage state out of the package folder.
	setEnv(t, "COMPOSE_FILE", t.TempDir()+"/"+composeTestFile)

	built := false
	var calledUp bool
	withComposeLifecycleDeps(t, []string{"a"}, func(d *core.AppDeps) {
		d.HostDeps.FetchText = func(string) (string, error) {
			return `{"data":{"2430930":{"depots":{"branches":{"public":{"buildid":"16491427"}}}}}}`, nil
		}
		d.Runtime.RunCommand = func([]string, ark.CommandOptions) (ark.ExecResult, error) { return ark.ExecResult{}, nil }
		d.DiskUsageForPath = func(string) (int64, int64, error) { return 100 << 30, 100 << 30, nil }
		d.Engine = &arktest.MockEngine{
			ImageExistsFn: func(_ context.Context, image string) (bool, error) {
				return false, nil
			},
			ReadFileFromImageFn: func(_ context.Context, image, path string) (string, error) {
				if strings.Contains(path, "VERSION") {
					return "82.3", nil
				}
				if strings.Contains(path, "BUILDID") {
					return "16491427", nil
				}
				return "", nil
			},
			BuildImageFn: func(_ context.Context, req ark.BuildRequest) error {
				built = true
				return nil
			},
		}
		d.ComposeService = &arktest.MockCompose{
			LoadProjectFn: func(_ context.Context, _ string) (*ark.ComposeProject, error) {
				return fakeProject(types.Services{"a": {Name: "a", Image: "ark-server:latest"}}), nil
			},
			UpFn: func(_ context.Context, _ string, _ []string, _ ark.UpOptions) error {
				calledUp = true
				return nil
			},
		}
		d.RunningServiceSnapshot = func(_ string) (map[string]struct{}, error) {
			return map[string]struct{}{"a": {}}, nil
		}
		d.HealthChecker = &mockHealthChecker{}
	}, func() {
		if err := compose.RunStart(core.CurrentAppDeps(), []string{"--instance", "a"}); err != nil {
			t.Fatalf("runStart: %v", err)
		}
	})
	if !built {
		t.Fatal("expected local build to run")
	}
	if !calledUp {
		t.Fatal("expected compose Up call")
	}
}

func TestRunStartNoWaitSkipsReadinessWait(t *testing.T) {
	setEnv(t, "COMPOSE_FILE", composeTestFile)

	waitCalled := false
	withComposeLifecycleDeps(t, []string{"a"}, func(d *core.AppDeps) {
		d.HealthChecker = &mockHealthChecker{
			waitReadyFn: func(_ context.Context, _, _ string, _ health.WaitOptions) error {
				waitCalled = true
				return nil
			},
		}
	}, func() {
		if err := compose.RunStart(core.CurrentAppDeps(), []string{"--instance", "a", "--no-wait"}); err != nil {
			t.Fatalf("runStart: %v", err)
		}
	})
	if waitCalled {
		t.Fatal("expected no readiness wait when --no-wait is set")
	}
}

func TestRunStartUpErrorPropagates(t *testing.T) {
	setEnv(t, "COMPOSE_FILE", composeTestFile)

	withComposeLifecycleDeps(t, []string{"a"}, func(d *core.AppDeps) {
		loadProject := d.ComposeService.(*arktest.MockCompose).LoadProjectFn
		d.ComposeService = &arktest.MockCompose{
			LoadProjectFn: loadProject,
			UpFn: func(_ context.Context, _ string, _ []string, _ ark.UpOptions) error {
				return errors.New("compose up failed")
			},
		}
	}, func() {
		err := compose.RunStart(core.CurrentAppDeps(), []string{"--instance", "a", "--no-wait"})
		if err == nil {
			t.Fatal("expected compose up error")
		}
		if !strings.Contains(err.Error(), "compose up failed") {
			t.Fatalf("expected error to propagate, got: %v", err)
		}
	})
}

func TestRunStartPassesDefaultAndCustomWaitTimeout(t *testing.T) {
	tests := []struct {
		name    string
		args    []string
		wantDur time.Duration
	}{
		{name: "default", args: []string{"--instance", "a"}, wantDur: 5 * time.Minute},
		{name: "custom", args: []string{"--instance", "a", "--wait-timeout", "45s"}, wantDur: 45 * time.Second},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			setEnv(t, "COMPOSE_FILE", composeTestFile)

			var gotTimeout time.Duration
			withComposeLifecycleDeps(t, []string{"a"}, func(d *core.AppDeps) {
				d.RunningServiceSnapshot = func(_ string) (map[string]struct{}, error) {
					return map[string]struct{}{"a": {}}, nil
				}
				d.HealthChecker = &mockHealthChecker{
					waitReadyFn: func(_ context.Context, _, _ string, opts health.WaitOptions) error {
						gotTimeout = opts.Timeout
						return nil
					},
				}
			}, func() {
				if err := compose.RunStart(core.CurrentAppDeps(), tc.args); err != nil {
					t.Fatalf("runStart: %v", err)
				}
			})

			if gotTimeout != tc.wantDur {
				t.Fatalf("unexpected wait timeout: got %s want %s", gotTimeout, tc.wantDur)
			}
		})
	}
}

func TestRunStartRejectsNonPositiveWaitTimeout(t *testing.T) {
	setEnv(t, "COMPOSE_FILE", composeTestFile)

	withComposeLifecycleDeps(t, []string{"a"}, nil, func() {
		if err := compose.RunStart(core.CurrentAppDeps(), []string{"--instance", "a", "--wait-timeout", "0s"}); err == nil {
			t.Fatal("expected error for non-positive wait timeout")
		}
	})
}

func TestRunStartUsesFreshReadyLogsForPreviouslyStoppedService(t *testing.T) {
	setEnv(t, "COMPOSE_FILE", composeTestFile)

	var sinceValues []*time.Time
	withComposeLifecycleDeps(t, []string{"a"}, func(d *core.AppDeps) {
		d.RunningServiceSnapshot = func(_ string) (map[string]struct{}, error) {
			return map[string]struct{}{}, nil // service NOT in pre-start snapshot
		}
		d.ComposeService = &arktest.MockCompose{
			LoadProjectFn: func(_ context.Context, _ string) (*ark.ComposeProject, error) {
				return fakeProject(composeTestServices([]string{"a"})), nil
			},
			RunningServicesFn: func(_ context.Context, _ string) (map[string]struct{}, error) {
				return map[string]struct{}{"a": {}}, nil
			},
			LogsFn: func(_ context.Context, _, _ string, opts ark.LogOptions) (io.ReadCloser, error) {
				sinceValues = append(sinceValues, opts.Since)
				return io.NopCloser(strings.NewReader(health.StartedMarker + "\n" + health.ReadyMarker)), nil
			},
		}
	}, func() {
		if err := compose.RunStart(core.CurrentAppDeps(), []string{"--instance", "a", "--wait-timeout", "3s"}); err != nil {
			t.Fatalf("runStart: %v", err)
		}
	})

	if len(sinceValues) == 0 {
		t.Fatal("expected readiness log polling")
	}
	foundSince := false
	for _, s := range sinceValues {
		if s != nil {
			foundSince = true
			break
		}
	}
	if !foundSince {
		t.Fatal("expected readiness log polling to use Since parameter")
	}
}

func TestRunStopImmediateUsesComposeKillAndPrintsMessage(t *testing.T) {
	var gotServices []string
	var gotSignal string
	out := captureStdout(t, func() {
		withComposeLifecycleDeps(t, []string{"a"}, func(d *core.AppDeps) {
			loadProject := d.ComposeService.(*arktest.MockCompose).LoadProjectFn
			d.ComposeService = &arktest.MockCompose{
				LoadProjectFn: loadProject,
				KillFn: func(_ context.Context, _ string, services []string, signal string) error {
					gotServices = services
					gotSignal = signal
					return nil
				},
			}
		}, func() {
			if err := compose.RunStop(core.CurrentAppDeps(), []string{"--instance", "a", "--kill"}); err != nil {
				t.Fatalf("runStop: %v", err)
			}
		})
	})

	if !strings.Contains(out, "bypassing graceful shutdown") {
		t.Fatalf("expected immediate message, got:\n%s", out)
	}
	if len(gotServices) != 1 || gotServices[0] != "a" {
		t.Fatalf("expected Kill called for service 'a', got %v", gotServices)
	}
	if gotSignal != "SIGKILL" {
		t.Fatalf("expected SIGKILL, got %s", gotSignal)
	}
}

func TestRunStopRejectsKillWithTimeout(t *testing.T) {
	if err := compose.RunStop(core.CurrentAppDeps(), []string{"--instance", "a", "--kill", "--timeout", "1m"}); err == nil {
		t.Fatal("expected error when combining --kill and --timeout")
	}
}

func TestRunDownSingleServiceLifecycle(t *testing.T) {
	tests := []struct {
		name         string
		running      bool
		hasContainer bool
		wantStop     bool
		wantRemove   bool
	}{
		{name: "running service is stopped then removed", running: true, hasContainer: true, wantStop: true, wantRemove: true},
		{name: "already down service is unchanged", running: false, hasContainer: false, wantStop: false, wantRemove: false},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			var stopped, removed bool
			withComposeLifecycleDeps(t, []string{"a"}, func(d *core.AppDeps) {
				loadProject := d.ComposeService.(*arktest.MockCompose).LoadProjectFn
				d.ComposeService = &arktest.MockCompose{
					LoadProjectFn: loadProject,
					RunningServicesFn: func(_ context.Context, _ string) (map[string]struct{}, error) {
						if tc.running {
							return map[string]struct{}{"a": {}}, nil
						}
						return map[string]struct{}{}, nil
					},
					StopFn: func(_ context.Context, _ string, _ []string, _ *time.Duration) error {
						stopped = true
						return nil
					},
					AllServicesFn: func(_ context.Context, _ string) (map[string]struct{}, error) {
						if tc.hasContainer {
							return map[string]struct{}{"a": {}}, nil
						}
						return map[string]struct{}{}, nil
					},
					RemoveFn: func(_ context.Context, _ string, _ []string) error {
						removed = true
						return nil
					},
				}
			}, func() {
				if err := compose.RunDownRequest(compose.DownCommandRequest{Instance: "a", ComposeFileFlag: composeTestFile}); err != nil {
					t.Fatalf("runDown: %v", err)
				}
			})

			if stopped != tc.wantStop {
				t.Fatalf("unexpected stop: got %t want %t", stopped, tc.wantStop)
			}
			if removed != tc.wantRemove {
				t.Fatalf("unexpected remove: got %t want %t", removed, tc.wantRemove)
			}
		})
	}
}

func TestRunDownAllErrorPropagates(t *testing.T) {
	withComposeLifecycleDeps(t, []string{"a", "b"}, func(d *core.AppDeps) {
		d.ComposeService = &arktest.MockCompose{
			LoadProjectFn: func(_ context.Context, _ string) (*ark.ComposeProject, error) {
				return fakeProject(composeTestServices([]string{"a", "b"})), nil
			},
			RunningServicesFn: func(_ context.Context, _ string) (map[string]struct{}, error) {
				return map[string]struct{}{}, nil
			},
			DownFn: func(_ context.Context, _ string) error {
				return errors.New("compose down failed")
			},
		}
	}, func() {
		err := compose.RunDownRequest(compose.DownCommandRequest{Instance: "all", ComposeFileFlag: composeTestFile})
		if err == nil {
			t.Fatal("expected compose down error")
		}
		if !strings.Contains(err.Error(), "compose down failed") {
			t.Fatalf("expected error to propagate, got: %v", err)
		}
	})
}

func TestRunDownSingleRemoveErrorPropagates(t *testing.T) {
	withComposeLifecycleDeps(t, []string{"a"}, func(d *core.AppDeps) {
		d.ComposeService = &arktest.MockCompose{
			LoadProjectFn: func(_ context.Context, _ string) (*ark.ComposeProject, error) {
				return fakeProject(composeTestServices([]string{"a"})), nil
			},
			RunningServicesFn: func(_ context.Context, _ string) (map[string]struct{}, error) {
				return map[string]struct{}{}, nil
			},
			AllServicesFn: func(_ context.Context, _ string) (map[string]struct{}, error) {
				return map[string]struct{}{"a": {}}, nil
			},
			RemoveFn: func(_ context.Context, _ string, _ []string) error {
				return errors.New("compose rm failed for service a")
			},
		}
	}, func() {
		err := compose.RunDownRequest(compose.DownCommandRequest{Instance: "a", ComposeFileFlag: composeTestFile})
		if err == nil {
			t.Fatal("expected compose rm error")
		}
		if !strings.Contains(err.Error(), "compose rm failed for service a") {
			t.Fatalf("expected error to propagate, got: %v", err)
		}
	})
}

func TestRunRemoveAllErrorsWithTeardownHint(t *testing.T) {
	err := remove.RunRemove(core.CurrentAppDeps(), []string{"--instance", "all"})
	if err == nil {
		t.Fatal("expected error for remove all")
	}
	if !strings.Contains(err.Error(), "teardown instances") {
		t.Fatalf("expected teardown hint in error, got: %v", err)
	}
}
