package cmd

// Tests: subcommand help output: canonical syntax display for flagset-based commands

import (
	"ark-cli/internal/backup"
	"ark-cli/internal/compose"
	"ark-cli/internal/core"
	"ark-cli/internal/create"
	"ark-cli/internal/health"
	"ark-cli/internal/host"
	"ark-cli/internal/ini"
	"ark-cli/internal/listenport"
	"ark-cli/internal/reachability"
	"ark-cli/internal/reconfigure"
	"ark-cli/internal/remove"
	"ark-cli/internal/schedule"
	"ark-cli/internal/show"
	"ark-cli/internal/update"
	"errors"
	"flag"
	"strings"
	"testing"
)

func TestSubcommandHelpShowsCanonicalSyntax(t *testing.T) {
	tests := []struct {
		name  string
		run   func([]string) error
		args  []string
		wants []string
	}{
		{
			name:  "create",
			run:   func(args []string) error { return create.RunCreate(core.CurrentAppDeps(), args) },
			args:  []string{"-h"},
			wants: []string{"create instance --name <name>"},
		},
		{
			name: "create schedule backup",
			run: func(args []string) error {
				return schedule.RunCreateSchedule(core.CurrentAppDeps(), append([]string{"backup"}, args...))
			},
			args:  []string{"-h"},
			wants: []string{"create schedule backup --instance <instance>"},
		},
		{
			name:  "edit ini",
			run:   func(args []string) error { return ini.RunEditIni(core.CurrentAppDeps(), args) },
			args:  []string{"-h"},
			wants: []string{"edit ini --instance <instance> --file <game|gameusersettings>"},
		},
		{
			name:  "reconfigure instance",
			run:   func(args []string) error { return reconfigure.RunReconfigure(core.CurrentAppDeps(), args) },
			args:  []string{"-h"},
			wants: []string{"reconfigure instance --instance <instance>"},
		},
		{
			name:  "update instance",
			run:   func(args []string) error { return update.RunUpdate(core.CurrentAppDeps(), args) },
			args:  []string{"-h"},
			wants: []string{"update instance [--instance <instance>]"},
		},
		{
			name:  "update schedule",
			run:   func(args []string) error { return schedule.RunUpdateSchedule(core.CurrentAppDeps(), args) },
			args:  []string{"-h"},
			wants: []string{"update schedule --name <name>"},
		},
		{
			name:  "apply schedules",
			run:   func(args []string) error { return schedule.RunApplySchedules(core.CurrentAppDeps(), args) },
			args:  []string{"-h"},
			wants: []string{"apply schedules"},
		},
		{
			name:  "start",
			run:   func(args []string) error { return compose.RunStart(core.CurrentAppDeps(), args) },
			args:  []string{"-h"},
			wants: []string{"start instance --instance <instance|all>"},
		},
		{
			name:  "restart instance",
			run:   func(args []string) error { return compose.RunRestart(core.CurrentAppDeps(), args) },
			args:  []string{"-h"},
			wants: []string{"restart instance --instance <instance|all>"},
		},
		{
			name:  "save instance",
			run:   func(args []string) error { return compose.RunSave(core.CurrentAppDeps(), args) },
			args:  []string{"-h"},
			wants: []string{"save instance --instance <instance|all>"},
		},
		{
			name:  "logs instance",
			run:   func(args []string) error { return compose.RunLogs(core.CurrentAppDeps(), args) },
			args:  []string{"-h"},
			wants: []string{"logs instance --instance <instance>"},
		},
		{
			name:  "check instance",
			run:   func(args []string) error { return health.RunCheckInstance(core.CurrentAppDeps(), args) },
			args:  []string{"-h"},
			wants: []string{"check instance --instance <instance>"},
		},
		{
			name:  "wait instance",
			run:   func(args []string) error { return health.RunWait(core.CurrentAppDeps(), args) },
			args:  []string{"-h"},
			wants: []string{"wait instance --instance <instance>"},
		},
		{
			name:  "validate compose",
			run:   func(args []string) error { return health.RunValidateCompose(core.CurrentAppDeps(), args) },
			args:  []string{"-h"},
			wants: []string{"validate compose"},
		},
		{
			name:  "validate schedule",
			run:   func(args []string) error { return schedule.RunValidateSchedule(core.CurrentAppDeps(), args) },
			args:  []string{"-h"},
			wants: []string{"validate schedule [--name <name>]"},
		},
		{
			name:  "validate backup",
			run:   func(args []string) error { return backup.RunVerifyBackup(core.CurrentAppDeps(), args) },
			args:  []string{"-h"},
			wants: []string{"validate backup --instance <instance> --backup <backup|latest>"},
		},
		{
			name:  "prune backups",
			run:   func(args []string) error { return backup.RunPruneBackups(core.CurrentAppDeps(), args) },
			args:  []string{"-h"},
			wants: []string{"prune backups --instance <instance|cluster|all>"},
		},
		{
			name:  "backup",
			run:   func(args []string) error { return backup.RunBackup(core.CurrentAppDeps(), args) },
			args:  []string{"-h"},
			wants: []string{"create backup --instance <instance|cluster|all>"},
		},
		{
			name:  "show instance",
			run:   func(args []string) error { return show.RunShowInstance(core.CurrentAppDeps(), args) },
			args:  []string{"-h"},
			wants: []string{"show instance --instance <instance>"},
		},
		{
			name:  "show backup",
			run:   func(args []string) error { return show.RunShowBackup(core.CurrentAppDeps(), args) },
			args:  []string{"-h"},
			wants: []string{"show backup --instance <instance> --backup <backup|latest>"},
		},
		{
			name:  "show compose",
			run:   func(args []string) error { return show.RunShowCompose(core.CurrentAppDeps(), args) },
			args:  []string{"-h"},
			wants: []string{"show compose"},
		},
		{
			name:  "show volume-paths",
			run:   func(args []string) error { return show.RunShowVolumePaths(core.CurrentAppDeps(), args) },
			args:  []string{"-h"},
			wants: []string{"show volume-paths --instance <instance>"},
		},
		{
			name:  "show schedule",
			run:   func(args []string) error { return schedule.RunShowSchedule(core.CurrentAppDeps(), args) },
			args:  []string{"-h"},
			wants: []string{"show schedule --name <name>"},
		},
		{
			name:  "run schedule",
			run:   func(args []string) error { return schedule.RunRunSchedule(core.CurrentAppDeps(), args) },
			args:  []string{"-h"},
			wants: []string{"run schedule --name <name>"},
		},
		{
			name:  "check host-kernel-config",
			run:   func(args []string) error { return host.RunCheckHostKernelConfig(core.CurrentAppDeps(), args) },
			args:  []string{"-h"},
			wants: []string{"check host-kernel-config"},
		},
		{
			name:  "fix host-kernel-config",
			run:   func(args []string) error { return host.RunFixHostKernelConfig(core.CurrentAppDeps(), args) },
			args:  []string{"-h"},
			wants: []string{"fix host-kernel-config"},
		},
		{
			name:  "restore host-kernel-config",
			run:   func(args []string) error { return host.RunRestoreHostKernelConfig(core.CurrentAppDeps(), args) },
			args:  []string{"-h"},
			wants: []string{"restore host-kernel-config"},
		},
		{
			name:  "restore",
			run:   func(args []string) error { return backup.RunRestore(core.CurrentAppDeps(), args) },
			args:  []string{"-h"},
			wants: []string{"restore backup --instance <name> --archive", "restore backup --new-instance <name> --archive", "restore backup --instance cluster --archive <backup|latest>"},
		},
		{
			name:  "remove",
			run:   func(args []string) error { return remove.RunRemove(core.CurrentAppDeps(), args) },
			args:  []string{"-h"},
			wants: []string{"remove instance --instance <instance>"},
		},
		{
			name:  "remove schedule",
			run:   func(args []string) error { return schedule.RunRemoveSchedule(core.CurrentAppDeps(), args) },
			args:  []string{"-h"},
			wants: []string{"remove schedule --name <name>"},
		},
		{
			name:  "check_reachability",
			run:   func(args []string) error { return reachability.RunCheckReachability(core.CurrentAppDeps(), args) },
			args:  []string{"-h"},
			wants: []string{"check reachability --port <num>"},
		},
		{
			name:  "listen_port",
			run:   func(args []string) error { return listenport.RunListenPort(core.CurrentAppDeps(), args) },
			args:  []string{"-h"},
			wants: []string{"listen port --port <num>"},
		},
		{
			name:  "list instances",
			run:   func(args []string) error { return show.RunListInstances(core.CurrentAppDeps(), args) },
			args:  []string{"-h"},
			wants: []string{"list instances"},
		},
		{
			name:  "list schedules",
			run:   func(args []string) error { return schedule.RunListSchedules(core.CurrentAppDeps(), args) },
			args:  []string{"-h"},
			wants: []string{"list schedules"},
		},
		{
			name:  "list backups",
			run:   func(args []string) error { return show.RunListBackups(core.CurrentAppDeps(), args) },
			args:  []string{"-h"},
			wants: []string{"list backups --instance <instance|cluster|all>"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var err error
			out := captureStderr(t, func() {
				err = tt.run(tt.args)
			})
			if !errors.Is(err, flag.ErrHelp) {
				t.Fatalf("expected flag.ErrHelp, got %v", err)
			}
			for _, want := range tt.wants {
				if !strings.Contains(out, want) {
					t.Fatalf("help output missing %q\noutput:\n%s", want, out)
				}
			}
			if strings.Contains(out, "Legacy:") {
				t.Fatalf("help output should not include legacy forms\noutput:\n%s", out)
			}
		})
	}
}
