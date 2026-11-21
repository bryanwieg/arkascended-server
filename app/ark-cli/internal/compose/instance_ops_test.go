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
	"pkg/ark/backup"
)

// Restart runs stop then start inside one span of the instance lock.
func TestRunRestartCallsStopThenStart(t *testing.T) {
	type call struct {
		name   string
		stop   *stopCommandRequest
		start  *startCommandRequest
		locked bool
	}
	backupRoot := t.TempDir()
	t.Setenv("ARK_BACKUP_DIR", backupRoot)
	locked := func() bool {
		held, err := backup.InstanceLockHeld(backup.InstanceLockDir(backupRoot, "asa1"), "asa1")
		return err == nil && held
	}
	calls := []call{}
	withPatchedAppDeps(t, func(d *core.AppDeps) {
		d.ComposeService = &arktest.MockCompose{
			LoadProjectFn: func(context.Context, string) (*ark.ComposeProject, error) {
				return fakeProject(emptyServices("asa1")), nil
			},
		}
		d.RestartStopInstance = func(req stopCommandRequest) error {
			copyReq := req
			calls = append(calls, call{name: "stop", stop: &copyReq, locked: locked()})
			return nil
		}
		d.RestartStartInstance = func(req startCommandRequest) error {
			copyReq := req
			calls = append(calls, call{name: "start", start: &copyReq, locked: locked()})
			return nil
		}
	}, func() {
		if err := RunRestart(core.CurrentAppDeps(), []string{"--instance", "asa1", "--timeout", "1m"}); err != nil {
			t.Fatalf("runRestart: %v", err)
		}
	})

	expected := []call{
		{name: "stop", stop: &stopCommandRequest{Instance: "asa1", Timeout: time.Minute}, locked: true},
		{name: "start", start: &startCommandRequest{Instance: "asa1", WaitTimeout: DefaultStartReadyWaitTimeout}, locked: true},
	}
	if !reflect.DeepEqual(calls, expected) {
		t.Fatalf("unexpected calls: %#v", calls)
	}
	if locked() {
		t.Fatal("instance lock still held after restart")
	}
}

// runStopWithLockProbe runs `stop instance` against a mock compose and reports whether the
// instance lock was held when the stop (or kill) reached compose.
func runStopWithLockProbe(t *testing.T, backupRoot string, args ...string) (lockedAtStop bool, out string, err error) {
	t.Helper()
	t.Setenv("ARK_BACKUP_DIR", backupRoot)
	probe := func(context.Context, string, []string) error { //nolint:unparam // must match the mock StopFn/KillFn signature
		held, probeErr := backup.InstanceLockHeld(backup.InstanceLockDir(backupRoot, "asa1"), "asa1")
		lockedAtStop = probeErr == nil && held
		return nil
	}
	withPatchedAppDeps(t, func(d *core.AppDeps) {
		d.ComposeService = &arktest.MockCompose{
			LoadProjectFn: func(context.Context, string) (*ark.ComposeProject, error) {
				return fakeProject(emptyServices("asa1")), nil
			},
			StopFn: func(ctx context.Context, f string, s []string, _ *time.Duration) error { return probe(ctx, f, s) },
			KillFn: func(ctx context.Context, f string, s []string, _ string) error { return probe(ctx, f, s) },
		}
	}, func() {
		out = captureStdout(t, func() { err = RunStop(core.CurrentAppDeps(), append([]string{"--instance", "asa1"}, args...)) })
	})
	return lockedAtStop, out, err
}

func TestRunStopHoldsInstanceLock(t *testing.T) {
	locked, _, err := runStopWithLockProbe(t, t.TempDir())
	if err != nil {
		t.Fatalf("RunStop: %v", err)
	}
	if !locked {
		t.Fatal("stop ran without the instance lock")
	}
}

// --kill is the way out of a hung operation, so it never waits behind that operation's lock.
func TestRunStopKillIgnoresHeldLock(t *testing.T) {
	setLockWaitTimeout(t, 200*time.Millisecond)
	backupRoot := t.TempDir()
	release, err := backup.AcquireInstanceLock(backup.InstanceLockDir(backupRoot, "asa1"), "asa1", 0)
	if err != nil {
		t.Fatal(err)
	}
	defer release()
	reachedKill, out, err := runStopWithLockProbe(t, backupRoot, "--kill") // true: the kill saw the other holder's lock
	if err != nil || !reachedKill {
		t.Fatalf("RunStop --kill: err=%v, kill reached compose=%v", err, reachedKill)
	}
	if strings.Contains(out, "Waiting for backup lock") {
		t.Fatalf("--kill waited for the lock:\n%s", out)
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
