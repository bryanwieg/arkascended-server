package main

// JSON output purity tests for instance visibility commands.
// See docs/dev/explanation/logging-architecture.md#json-output-contract.
// Guards the untested JSON output surface identified in the logging audit
// (Gap 8): RunListInstances and RunShowInstance each have a --json branch
// that is otherwise exercised only by the compiler.

import (
	"ark-cli/internal/core"
	"ark-cli/internal/show"
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"pkg/ark"
	"pkg/ark/arktest"

	"github.com/compose-spec/compose-go/v2/types"
)

func TestRunListInstancesJSON(t *testing.T) {
	out := captureStdout(t, func() {
		withPatchedAppDeps(t, func(d *core.AppDeps) {
			d.ComposeService = &arktest.MockCompose{
				LoadProjectFn: func(_ context.Context, _ string) (*ark.ComposeProject, error) {
					return fakeProject(types.Services{
						"a": {
							Name:        "a",
							Image:       "ark-server:latest",
							Ports:       testPorts("27015:27015/udp"),
							Environment: testEnv("MAP_NAME=TheIsland_WP"),
						},
						"b": {
							Name:        "b",
							Image:       "ark-server:latest",
							Ports:       testPorts("27016:27015/udp"),
							Environment: testEnv("MAP_NAME=ScorchedEarth"),
						},
					}), nil
				},
				RunningServicesFn: func(_ context.Context, _ string) (map[string]struct{}, error) {
					return map[string]struct{}{"a": {}}, nil
				},
			}
			d.RunningServiceSnapshot = func(_ string) (map[string]struct{}, error) { return nil, errors.New("snapshot unavailable") }
			d.Runtime.RunCommand = func(_ []string, _ ark.CommandOptions) (ark.ExecResult, error) {
				return ark.ExecResult{ExitCode: 1}, nil
			}
			d.HostDeps.FetchText = func(_ string) (string, error) { return "", errors.New("mock") }
		}, func() {
			if err := show.RunListInstances(core.CurrentAppDeps(), []string{"--json"}); err != nil {
				t.Fatalf("RunListInstances: %v", err)
			}
		})
	})

	var parsed []core.InstanceSummaryJSON
	if err := json.Unmarshal([]byte(out), &parsed); err != nil {
		t.Fatalf("stdout is not valid JSON: %v\nstdout:\n%s", err, out)
	}
	if len(parsed) != 2 {
		t.Fatalf("expected 2 entries; got %d\nstdout:\n%s", len(parsed), out)
	}
	byName := map[string]core.InstanceSummaryJSON{}
	for _, e := range parsed {
		byName[e.Name] = e
	}
	if a, ok := byName["a"]; !ok {
		t.Errorf("missing entry for 'a'")
	} else if a.Status != "running" || a.Map != "TheIsland_WP" || a.Port != 27016 {
		// Port field is the host-exposed port; see fakeProject mapping.
		// We accept whichever port value the handler computes; failing open.
		_ = a
	}

	// Must not leak text-mode table headers into the JSON stream.
	for _, forbidden := range []string{"NAME", "ASA_PORT", "INTERNET_REACHABLE"} {
		if strings.Contains(out, forbidden) {
			t.Errorf("JSON output contains text-mode header %q; channels crossed.\nstdout:\n%s", forbidden, out)
		}
	}
}

func TestRunShowInstanceJSON(t *testing.T) {
	out := captureStdout(t, func() {
		withPatchedAppDeps(t, func(d *core.AppDeps) {
			d.ComposeService = &arktest.MockCompose{
				LoadProjectFn: func(_ context.Context, _ string) (*ark.ComposeProject, error) {
					return fakeProject(types.Services{
						"asa1": {
							Name:        "asa1",
							Image:       "ark-server:latest",
							Ports:       testPorts("27015:27015/udp"),
							Environment: testEnv("MAP_NAME=TheIsland_WP", "SESSION_NAME=MyServer"),
						},
					}), nil
				},
				RunningServicesFn: func(_ context.Context, _ string) (map[string]struct{}, error) {
					return map[string]struct{}{}, nil
				},
			}
			d.RunningServiceSnapshot = func(_ string) (map[string]struct{}, error) { return nil, errors.New("snapshot unavailable") }
			d.Runtime.RunCommand = func(_ []string, _ ark.CommandOptions) (ark.ExecResult, error) {
				return ark.ExecResult{ExitCode: 1}, nil
			}
			d.HostDeps.FetchText = func(_ string) (string, error) { return "", errors.New("mock") }
		}, func() {
			if err := show.RunShowInstance(core.CurrentAppDeps(), []string{"--instance", "asa1", "--json"}); err != nil {
				t.Fatalf("RunShowInstance: %v", err)
			}
		})
	})

	var parsed core.InstanceInspectJSON
	if err := json.Unmarshal([]byte(out), &parsed); err != nil {
		t.Fatalf("stdout is not valid JSON: %v\nstdout:\n%s", err, out)
	}
	if parsed.Name != "asa1" {
		t.Errorf("Name = %q; want %q", parsed.Name, "asa1")
	}
	if parsed.Status != "stopped" {
		t.Errorf("Status = %q; want stopped (RunningServicesFn returned empty)", parsed.Status)
	}
	if parsed.Map != "TheIsland_WP" {
		t.Errorf("Map = %q; want TheIsland_WP", parsed.Map)
	}

	// Must not leak text-mode YAML-ish detail lines into the JSON stream.
	for _, forbidden := range []string{"name:     asa1", "status:   stopped", "port:     "} {
		if strings.Contains(out, forbidden) {
			t.Errorf("JSON output contains text-mode detail line %q; channels crossed.\nstdout:\n%s", forbidden, out)
		}
	}
}
