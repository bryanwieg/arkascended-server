package backup

import (
	"context"
	"log/slog"
	"strings"
	"testing"

	"ark-cli/internal/core"

	"pkg/ark"
	"pkg/ark/arktest"
	pkgbackup "pkg/ark/backup"
	"pkg/ark/cmdrunner"

	"github.com/compose-spec/compose-go/v2/types"
)

func TestRunBackupParsesFlagsAndCallsCreateForTarget(t *testing.T) {
	tmp := t.TempDir()
	setEnv(t, "ARK_BACKUP_DIR", tmp)
	setEnv(t, "COMPOSE_FILE", "docker-compose.yaml")

	instance := "asa1"
	var execCalled bool
	composeMock := &arktest.MockCompose{
		LoadProjectFn: func(_ context.Context, _ string) (*ark.ComposeProject, error) {
			return fakeProject(types.Services{instance: {Name: instance}}), nil
		},
		RunningServicesFn: func(_ context.Context, _ string) (map[string]struct{}, error) {
			return map[string]struct{}{instance: {}}, nil
		},
		ExecFn: func(_ context.Context, _, _ string, _ []string, _ ark.ExecOptions) (ark.ExecResult, error) {
			execCalled = true
			return ark.ExecResult{ExitCode: 0}, nil
		},
		CopyFromServiceFn: func(_ context.Context, _, _, _, dstPath string) error {
			return nil
		},
	}
	runner := &arktest.MockCommandRunner{
		RunFn: func(_ context.Context, _ []string, _ cmdrunner.RunOptions) (cmdrunner.RunResult, error) {
			return cmdrunner.RunResult{ExitCode: 0, Stdout: "ok"}, nil
		},
	}
	bs := pkgbackup.New(nil, composeMock, nil, runner, slog.Default())
	bs.DiskUsage = func(_ string) (int64, int64, error) {
		return pkgbackup.MinFreeBackupBytes + 1024, 0, nil
	}

	withPatchedAppDeps(t, func(d *core.AppDeps) {
		d.ComposeService = composeMock
		d.BackupService = bs
	}, func() {
		err := RunBackup(core.CurrentAppDeps(), []string{"--instance", instance})
		if err != nil {
			t.Fatalf("RunBackup: %v", err)
		}
		if !execCalled {
			t.Fatal("expected Exec to be called for running service backup")
		}
	})
}

func TestRunBackupMissingInstance(t *testing.T) {
	tmp := t.TempDir()
	setEnv(t, "ARK_BACKUP_DIR", tmp)
	setEnv(t, "COMPOSE_FILE", "docker-compose.yaml")

	withPatchedAppDeps(t, func(d *core.AppDeps) {
		d.ComposeService = &arktest.MockCompose{
			LoadProjectFn: func(_ context.Context, _ string) (*ark.ComposeProject, error) {
				return fakeProject(types.Services{"other": {Name: "other"}}), nil
			},
		}
	}, func() {
		err := RunBackup(core.CurrentAppDeps(), []string{"--instance", "asa1"})
		if err == nil {
			t.Fatal("expected error for missing instance")
		}
		if !strings.Contains(err.Error(), "compose service not found") {
			t.Fatalf("unexpected error: %v", err)
		}
	})
}

func TestRunBackupCustomName(t *testing.T) {
	req, err := ParseBackupCreateCommandRequest([]string{"--instance", "asa1", "--name", "manual-name"})
	if err != nil {
		t.Fatalf("ParseBackupCreateCommandRequest: %v", err)
	}
	if req.Name != "manual-name" {
		t.Fatalf("expected name 'manual-name', got %q", req.Name)
	}
}

func TestRunBackupInvalidFormat(t *testing.T) {
	err := RunBackup(nil, []string{"--instance", "asa1", "--format", "invalid"})
	if err == nil {
		t.Fatal("expected error for invalid format")
	}
	if !strings.Contains(err.Error(), pkgbackup.FormatValidationError) {
		t.Fatalf("unexpected error: %v", err)
	}
}
