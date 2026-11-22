package main

// Tests: instance health and wait commands: check instance result, wait for ready/stopped, validate compose

import (
	"ark-cli/internal/core"
	"ark-cli/internal/health"
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"pkg/ark"
	"pkg/ark/arktest"
	pkghealth "pkg/ark/health"

	"github.com/compose-spec/compose-go/v2/types"
)

func withInstanceHealthDeps(
	t *testing.T,
	services types.Services,
	mutate func(*core.AppDeps),
	run func(),
) {
	t.Helper()
	withPatchedAppDeps(t, func(d *core.AppDeps) {
		d.ComposeService = &arktest.MockCompose{
			LoadProjectFn: func(_ context.Context, _ string) (*ark.ComposeProject, error) {
				return fakeProject(services), nil
			},
		}
		d.HealthChecker = &mockHealthChecker{}
		if mutate != nil {
			mutate(d)
		}
	}, run)
}

func TestRunCheckInstanceReportsHealthy(t *testing.T) {
	out := captureStdout(t, func() {
		withInstanceHealthDeps(t, types.Services{
			"asa1": {
				Name:        "asa1",
				Labels:      types.Labels{ark.PortASALabel: "27015"},
				Image:       "ark-server:latest",
				Ports:       testPorts("27015:27015/udp"),
				Environment: testEnv("MAP_NAME=TheIsland_WP"),
			},
		}, func(d *core.AppDeps) {
			d.ComposeService.(*arktest.MockCompose).RunningServicesFn = func(_ context.Context, _ string) (map[string]struct{}, error) {
				return map[string]struct{}{"asa1": {}}, nil
			}
			d.HealthChecker = &mockHealthChecker{
				isReadyFn: func(_ context.Context, _, _ string, _ *time.Time) (bool, error) { return true, nil },
			}
		}, func() {
			if err := health.RunCheckInstance(core.CurrentAppDeps(), []string{"--instance", "asa1"}); err != nil {
				t.Fatalf("runCheckInstance: %v", err)
			}
		})
	})
	for _, want := range []string{"name: asa1", "status: running", "ready: yes", "result: healthy"} {
		if !strings.Contains(out, want) {
			t.Fatalf("missing %q in output:\n%s", want, out)
		}
	}
}

func TestRunCheckInstanceFailsWhenRunningButNotReady(t *testing.T) {
	withInstanceHealthDeps(t, emptyServices("asa1"), func(d *core.AppDeps) {
		d.ComposeService.(*arktest.MockCompose).RunningServicesFn = func(_ context.Context, _ string) (map[string]struct{}, error) {
			return map[string]struct{}{"asa1": {}}, nil
		}
		d.HealthChecker = &mockHealthChecker{
			isReadyFn: func(_ context.Context, _, _ string, _ *time.Time) (bool, error) { return false, nil },
		}
	}, func() {
		err := health.RunCheckInstance(core.CurrentAppDeps(), []string{"--instance", "asa1"})
		if err == nil {
			t.Fatal("expected error for running but not ready")
		}
		if !strings.Contains(err.Error(), "starting") {
			t.Fatalf("expected 'starting' in error, got: %v", err)
		}
	})
}

func TestRunCheckInstanceSucceedsWhenStopped(t *testing.T) {
	out := captureStdout(t, func() {
		withInstanceHealthDeps(t, emptyServices("asa1"), func(d *core.AppDeps) {
			d.ComposeService.(*arktest.MockCompose).RunningServicesFn = func(_ context.Context, _ string) (map[string]struct{}, error) {
				return map[string]struct{}{}, nil
			}
			d.HealthChecker = &mockHealthChecker{
				isReadyFn: func(_ context.Context, _, _ string, _ *time.Time) (bool, error) { return false, nil },
			}
		}, func() {
			if err := health.RunCheckInstance(core.CurrentAppDeps(), []string{"--instance", "asa1"}); err != nil {
				t.Fatalf("expected success for stopped instance, got: %v", err)
			}
		})
	})
	if !strings.Contains(out, "result: stopped") {
		t.Fatalf("expected 'result: stopped' in output:\n%s", out)
	}
}

func TestRunWaitImmediateSuccess(t *testing.T) {
	tests := []struct {
		name      string
		waitFor   string
		wantText  string
		configure func(*core.AppDeps)
	}{
		{
			name:     "started",
			waitFor:  "started",
			wantText: "started",
			configure: func(d *core.AppDeps) {
				d.HealthChecker = &mockHealthChecker{
					waitStartedFn: func(_ context.Context, _, _ string, _ pkghealth.WaitOptions) error { return nil },
					waitReadyFn: func(_ context.Context, _, _ string, _ pkghealth.WaitOptions) error {
						return errors.New("unexpected ready path")
					},
					waitStoppedFn: func(_ context.Context, _, _ string, _ pkghealth.WaitOptions) error {
						return errors.New("unexpected stopped path")
					},
				}
			},
		},
		{
			name:     "advertising",
			waitFor:  "advertising",
			wantText: "advertising",
			configure: func(d *core.AppDeps) {
				d.HealthChecker = &mockHealthChecker{
					waitReadyFn: func(_ context.Context, _, _ string, _ pkghealth.WaitOptions) error { return nil },
					waitStartedFn: func(_ context.Context, _, _ string, _ pkghealth.WaitOptions) error {
						return errors.New("unexpected started path")
					},
					waitStoppedFn: func(_ context.Context, _, _ string, _ pkghealth.WaitOptions) error {
						return errors.New("unexpected stopped path")
					},
				}
			},
		},
		{
			name:     "ready (alias for advertising)",
			waitFor:  "ready",
			wantText: "advertising",
			configure: func(d *core.AppDeps) {
				d.HealthChecker = &mockHealthChecker{
					waitReadyFn: func(_ context.Context, _, _ string, _ pkghealth.WaitOptions) error { return nil },
					waitStartedFn: func(_ context.Context, _, _ string, _ pkghealth.WaitOptions) error {
						return errors.New("unexpected started path")
					},
					waitStoppedFn: func(_ context.Context, _, _ string, _ pkghealth.WaitOptions) error {
						return errors.New("unexpected stopped path")
					},
				}
			},
		},
		{
			name:     "stopped",
			waitFor:  "stopped",
			wantText: "stopped",
			configure: func(d *core.AppDeps) {
				d.HealthChecker = &mockHealthChecker{
					waitStoppedFn: func(_ context.Context, _, _ string, _ pkghealth.WaitOptions) error { return nil },
					waitReadyFn: func(_ context.Context, _, _ string, _ pkghealth.WaitOptions) error {
						return errors.New("unexpected ready path")
					},
					waitStartedFn: func(_ context.Context, _, _ string, _ pkghealth.WaitOptions) error {
						return errors.New("unexpected started path")
					},
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			out := captureStdout(t, func() {
				withInstanceHealthDeps(t, emptyServices("asa1"), tt.configure, func() {
					if err := health.RunWait(core.CurrentAppDeps(), []string{"--instance", "asa1", "--for", tt.waitFor, "--timeout", "1s", "--interval", "10ms"}); err != nil {
						t.Fatalf("runWait: %v", err)
					}
				})
			})
			if !strings.Contains(out, tt.wantText) {
				t.Fatalf("unexpected output: %s", out)
			}
		})
	}
}

func TestRunValidateComposeReportsServiceCount(t *testing.T) {
	out := captureStdout(t, func() {
		withInstanceHealthDeps(t, emptyServices("a", "b"), nil, func() {
			if err := health.RunValidateCompose(core.CurrentAppDeps(), []string{}); err != nil {
				t.Fatalf("runValidateCompose: %v", err)
			}
		})
	})
	if !strings.Contains(out, "2 service(s)") {
		t.Fatalf("unexpected output: %s", out)
	}
}
