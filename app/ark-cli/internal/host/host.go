package host

import (
	"ark-cli/internal/core"
	"errors"
	"fmt"
	"os"

	"pkg/ark"
	"pkg/ark/host"
)

// RunCheckHostKernelConfig reports the status of all required host kernel settings.
func RunCheckHostKernelConfig(d *core.AppDeps, args []string) error {
	fs := core.NewCommandFlagSet(
		"check_host_kernel_config",
		[]string{"check host-kernel-config"},
		"Check host-level kernel settings required for Proton/Wine.",
	)
	if err := core.ParseNoPositionalArgs(fs, args); err != nil {
		return err
	}

	var rows []host.KernelStatusRow
	anyFail := false

	for _, p := range host.HostKernelParams {
		ok, val, err := host.CheckOneParam(d.HostDeps, p)
		min := fmt.Sprintf("%d", p.Minimum)
		var cur, status string
		if err != nil {
			cur, status, anyFail = fmt.Sprintf("error: %v", err), "error", true
		} else if ok {
			cur, status = fmt.Sprintf("%d", val), "OK"
		} else {
			cur, status, anyFail = fmt.Sprintf("%d", val), "FAIL", true
		}
		rows = append(rows, host.KernelStatusRow{Name: p.Name, Col1: cur, Col2: min, Status: status})
	}

	nofileOK, nofileVal, nofileErr := host.CheckNofileLimit(d.HostDeps)
	nofileMin := fmt.Sprintf("%d", host.MinNofileHardLimit)
	var nofileCur, nofileStatus string
	if nofileErr != nil {
		nofileCur, nofileStatus, anyFail = fmt.Sprintf("error: %v", nofileErr), "error", true
	} else if nofileOK {
		nofileCur, nofileStatus = fmt.Sprintf("%d", nofileVal), "OK"
	} else {
		nofileCur, nofileStatus, anyFail = fmt.Sprintf("%d", nofileVal), "FAIL", true
	}
	rows = append(rows, host.KernelStatusRow{Name: host.NofileSettingName, Col1: nofileCur, Col2: nofileMin, Status: nofileStatus})

	host.PrintKernelStatusTable(d.HostDeps.Stdout, "current", "minimum", rows)

	_, confConflicts := host.ScanSysctlConf()
	if len(confConflicts) > 0 {
		core.Infof("\nNote: /etc/sysctl.conf overrides the following on reboot:\n")
		for _, c := range confConflicts {
			core.Infof("  %s=%d (minimum: %d)\n", c.Name, c.OldVal, c.NewVal)
		}
	}
	if anyFail {
		core.Infof("\nRun 'sudo ark-cli fix host-kernel-config' to apply required settings.\n")
	}
	return nil
}

// RunFixHostKernelConfig applies and persists all required host kernel settings.
func RunFixHostKernelConfig(d *core.AppDeps, args []string) error {
	fs := core.NewCommandFlagSet(
		"fix_host_kernel_config",
		[]string{"fix host-kernel-config"},
		"Apply and persist host-level kernel settings required for Proton/Wine (requires root).",
	)
	if err := core.ParseNoPositionalArgs(fs, args); err != nil {
		return err
	}
	if err := host.RequireRoot("fix host-kernel-config"); err != nil {
		return err
	}

	snapshot, err := host.SnapshotKernelParams(d.HostDeps)
	if err != nil {
		return fmt.Errorf("Error: %w", err)
	}
	nofileOld, err := d.HostDeps.GetNofileHardLimit()
	if err != nil {
		return fmt.Errorf("Error: could not read nofile hard limit: %w", err)
	}

	if backupPath, bErr := host.KernelParamBackupPath(); bErr == nil {
		if wErr := host.WriteKernelParamBackup(snapshot, nofileOld, backupPath); wErr != nil {
			core.Warnf("Warning: could not write backup file: %v\n", wErr)
		} else {
			core.Infof("Backup of original values saved to: %s\n", backupPath)
		}
	}

	var rows []host.KernelStatusRow
	sysctlChanged := 0

	for _, p := range host.HostKernelParams {
		oldVal := snapshot[p.Name]
		before, after := fmt.Sprintf("%d", oldVal), fmt.Sprintf("%d", p.Minimum)
		if oldVal >= p.Minimum {
			rows = append(rows, host.KernelStatusRow{Name: p.Name, Col1: before, Col2: after, Status: "ok (skipped)"})
			continue
		}
		if err := core.RunCommandExpectZero(d,
			[]string{"sysctl", "-w", fmt.Sprintf("%s=%d", p.Name, p.Minimum)},
			ark.CommandOptions{Capture: true}, nil,
		); err != nil {
			return err
		}
		rows = append(rows, host.KernelStatusRow{Name: p.Name, Col1: before, Col2: after, Status: "fixed"})
		sysctlChanged++
	}

	// Use the limits.d file to determine nofile status — root's own rlimit is
	// typically RLIM_INFINITY and would give a false "ok" reading.
	nofileAfter := fmt.Sprintf("%d", host.MinNofileHardLimit)
	configuredVal, nofileOK := host.NofileConfiguredValue()
	var nofileBefore string
	if configuredVal > 0 {
		nofileBefore = fmt.Sprintf("%d", configuredVal)
	} else {
		nofileBefore = "not set"
	}
	if nofileOK {
		rows = append(rows, host.KernelStatusRow{Name: host.NofileSettingName, Col1: nofileBefore, Col2: nofileAfter, Status: "ok (skipped)"})
	} else {
		if err := host.ApplyNofilePersistFiles(d.HostDeps); err != nil {
			return fmt.Errorf("Error: could not write nofile config: %w", err)
		}
		rows = append(rows, host.KernelStatusRow{Name: host.NofileSettingName, Col1: nofileBefore, Col2: nofileAfter, Status: "pending restart"})
	}

	if err := os.WriteFile(host.SysctlPersistPath, []byte(host.SysctlPersistContent()), 0o644); err != nil {
		return fmt.Errorf("Error: could not write %s: %w", host.SysctlPersistPath, err)
	}

	// Patch /etc/sysctl.conf if it has conflicting values that would override on reboot.
	confConflicts, patchErr := host.PatchSysctlConf()
	if patchErr != nil {
		core.Warnf("Warning: could not patch %s: %v\n", host.SysctlConfPath, patchErr)
	}
	if len(confConflicts) > 0 {
		if backupPath, bErr := host.KernelParamBackupPath(); bErr == nil {
			_ = host.AppendSysctlConfToBackup(backupPath, confConflicts)
		}
	}

	host.PrintKernelStatusTable(d.HostDeps.Stdout, "before", "after", rows)
	core.Infof("\nPermanent sysctl config written to: %s\n", host.SysctlPersistPath)
	if len(confConflicts) > 0 {
		core.Infof("Patched conflicting values in %s:\n", host.SysctlConfPath)
		for _, c := range confConflicts {
			core.Infof("  %s: %d -> %d\n", c.Name, c.OldVal, c.NewVal)
		}
	}

	if sysctlChanged == 0 && nofileOK {
		core.Infoln("All settings already met required minimums (permanent configs updated).")
		return nil
	}
	if sysctlChanged > 0 {
		core.Infoln("Sysctl changes are live immediately — no restart required.")
	}
	if !nofileOK {
		core.Infoln("Open file limit (nofile) changes require a restart to take effect:")
		core.Infoln("  Docker daemon: sudo systemctl daemon-reexec")
		core.Infoln("  Shell session: log out and back in")
	}
	return nil
}

// RunRestoreHostKernelConfig reverts all host kernel settings from the pre-fix backup.
func RunRestoreHostKernelConfig(d *core.AppDeps, args []string) error {
	fs := core.NewCommandFlagSet(
		"restore_host_kernel_config",
		[]string{"restore host-kernel-config"},
		"Restore host kernel settings from the backup saved by 'fix host-kernel-config' (requires root).",
	)
	if err := core.ParseNoPositionalArgs(fs, args); err != nil {
		return err
	}
	if err := host.RequireRoot("restore host-kernel-config"); err != nil {
		return err
	}

	backupPath, err := host.KernelParamBackupPath()
	if err != nil {
		return fmt.Errorf("Error: could not resolve backup path: %w", err)
	}
	data, err := os.ReadFile(backupPath)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return errors.New("Error: no backup found. Run 'sudo ark-cli fix host-kernel-config' first.")
		}
		return fmt.Errorf("Error: could not read backup: %w", err)
	}
	values := host.ParseKernelBackupFile(data)
	if len(values) == 0 {
		return errors.New("Error: backup file is empty or contains no valid settings.")
	}

	snapshot, _ := host.SnapshotKernelParams(d.HostDeps)

	var rows []host.KernelStatusRow

	for _, p := range host.HostKernelParams {
		before := fmt.Sprintf("%d", snapshot[p.Name])
		val, ok := values[p.Name]
		if !ok {
			rows = append(rows, host.KernelStatusRow{Name: p.Name, Col1: before, Col2: "—", Status: "not in backup"})
			continue
		}
		if err := core.RunCommandExpectZero(d,
			[]string{"sysctl", "-w", fmt.Sprintf("%s=%s", p.Name, val)},
			ark.CommandOptions{Capture: true}, nil,
		); err != nil {
			return err
		}
		rows = append(rows, host.KernelStatusRow{Name: p.Name, Col1: before, Col2: val, Status: "restored"})
	}

	// Use the limits.d file for "before" — root's rlimit is typically RLIM_INFINITY.
	configuredVal, _ := host.NofileConfiguredValue()
	var nofileBefore string
	if configuredVal > 0 {
		nofileBefore = fmt.Sprintf("%d", configuredVal)
	} else {
		nofileBefore = "not set"
	}
	if nofileVal, ok := values[host.BackupNofileKey]; ok {
		rows = append(rows, host.KernelStatusRow{Name: host.NofileSettingName, Col1: nofileBefore, Col2: nofileVal, Status: "pending restart"})
	} else {
		rows = append(rows, host.KernelStatusRow{Name: host.NofileSettingName, Col1: nofileBefore, Col2: "—", Status: "not in backup"})
	}

	if err := os.Remove(host.SysctlPersistPath); err != nil && !errors.Is(err, os.ErrNotExist) {
		core.Warnf("Warning: could not remove %s: %v\n", host.SysctlPersistPath, err)
	}
	host.RevertSysctlConf(values)
	host.RemoveNofilePersistFiles(d.HostDeps)

	host.PrintKernelStatusTable(d.HostDeps.Stdout, "before", "after", rows)
	core.Infoln("\nSysctl settings restored immediately.")
	core.Infoln("Open file limit (nofile) restore requires a restart to take effect:")
	core.Infoln("  Docker daemon: sudo systemctl daemon-reexec")
	core.Infoln("  Shell session: log out and back in")
	return nil
}
