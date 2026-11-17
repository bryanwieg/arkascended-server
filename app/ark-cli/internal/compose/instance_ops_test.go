package compose

// Tests: instance lifecycle ops: restart stop-then-start sequencing, saveworld exec, logs command argument building

import (
	"ark-cli/internal/core"
	"context"
	"io"
	"reflect"
	"strings"
	"testing"
	"time"

	"pkg/ark"
	"pkg/ark/arktest"
)

func TestRunRestartCallsStopThenStart(t *testing.T) {
	type call struct {
		name  string
		stop  *stopCommandRequest
		start *startCommandRequest
	}
	calls := []call{}
	withPatchedAppDeps(t, func(d *core.AppDeps) {
		d.RestartStopInstance = func(req stopCommandRequest) error {
			copyReq := req
			calls = append(calls, call{name: "stop", stop: &copyReq})
			return nil
		}
		d.RestartStartInstance = func(req startCommandRequest) error {
			copyReq := req
			calls = append(calls, call{name: "start", start: &copyReq})
			return nil
		}
	}, func() {
		if err := RunRestart(core.CurrentAppDeps(), []string{"--instance", "asa1", "--timeout", "1m"}); err != nil {
			t.Fatalf("runRestart: %v", err)
		}
	})

	expected := []call{
		{name: "stop", stop: &stopCommandRequest{Instance: "asa1", Timeout: time.Minute}},
		{name: "start", start: &startCommandRequest{Instance: "asa1", WaitTimeout: DefaultStartReadyWaitTimeout}},
	}
	if !reflect.DeepEqual(calls, expected) {
		t.Fatalf("unexpected calls: %#v", calls)
	}
}

func TestRunSaveExecutesSaveworldForAllServices(t *testing.T) {
	type execCall struct {
		service string
		command []string
	}
	var execCalls []execCall
	withPatchedAppDeps(t, func(d *core.AppDeps) {
		d.ComposeService = &arktest.MockCompose{
			LoadProjectFn: func(_ context.Context, _ string) (*ark.ComposeProject, error) {
				return fakeProject(emptyServices("a", "b")), nil
			},
			ExecFn: func(_ context.Context, _, service string, command []string, _ ark.ExecOptions) (ark.ExecResult, error) {
				execCalls = append(execCalls, execCall{service: service, command: command})
				return ark.ExecResult{ExitCode: 0}, nil
			},
		}
	}, func() {
		if err := RunSave(core.CurrentAppDeps(), []string{"--instance", "all"}); err != nil {
			t.Fatalf("runSave: %v", err)
		}
	})
	if len(execCalls) != 2 {
		t.Fatalf("expected 2 exec calls, got %d", len(execCalls))
	}
	for i, service := range []string{"a", "b"} {
		if execCalls[i].service != service {
			t.Fatalf("expected exec on service %s, got %s", service, execCalls[i].service)
		}
		command := strings.Join(execCalls[i].command, " ")
		if !strings.HasSuffix(command, "ark-rcon.py saveworld") {
			t.Fatalf("expected ark-rcon.py saveworld, got %v", execCalls[i].command)
		}
		// The password must stay in the container env, never on the exec'd command line.
		if strings.Contains(command, "PASSWORD") || strings.Contains(command, " -p ") {
			t.Fatalf("password leaked into the command line: %v", execCalls[i].command)
		}
	}
}

func TestRunLogsPassesOptions(t *testing.T) {
	var gotService string
	var gotOpts ark.LogOptions
	withPatchedAppDeps(t, func(d *core.AppDeps) {
		d.ComposeService = &arktest.MockCompose{
			LoadProjectFn: func(_ context.Context, _ string) (*ark.ComposeProject, error) {
				return fakeProject(emptyServices("asa1")), nil
			},
			LogsFn: func(_ context.Context, _, service string, opts ark.LogOptions) (io.ReadCloser, error) {
				gotService = service
				gotOpts = opts
				return io.NopCloser(strings.NewReader("")), nil
			},
		}
	}, func() {
		if err := RunLogs(core.CurrentAppDeps(), []string{"--instance", "asa1", "--tail", "42", "--timestamps"}); err != nil {
			t.Fatalf("runLogs: %v", err)
		}
	})

	if gotService != "asa1" {
		t.Fatalf("unexpected service: %s", gotService)
	}
	if gotOpts.Tail != 42 {
		t.Fatalf("unexpected tail: %d", gotOpts.Tail)
	}
	if !gotOpts.Timestamps {
		t.Fatal("expected timestamps enabled")
	}
}
