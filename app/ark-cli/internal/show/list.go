package show

import (
	"ark-cli/internal/compose"
	"ark-cli/internal/core"
	"errors"
	"fmt"
	"os"
	"path/filepath"

	backup "pkg/ark/backup"
	"pkg/ark/host"
	"pkg/ark/runtime"
)

// RunListInstances lists instances with status and key health indicators.
func RunListInstances(d *core.AppDeps, args []string) error {
	fs := core.NewCommandFlagSet(
		"list instances",
		[]string{"list instances [options]"},
		"List instances with status and key health indicators.",
	)
	composeFileFlag := fs.String("compose-file", "", "Path to docker-compose.yaml (default: ./docker-compose.yaml or COMPOSE_FILE).")
	jsonFlag := fs.Bool("json", false, "Output as JSON.")
	if err := core.ParseNoPositionalArgs(fs, args); err != nil {
		return err
	}

	composeFile, config, err := compose.ResolveRequiredComposeConfig(d, *composeFileFlag)
	if err != nil {
		return err
	}
	summaries := compose.ListInstanceSummaries(d, composeFile, config)
	if len(summaries) == 0 {
		return errors.New("Error: compose config has no services.")
	}

	// Resolve public IP once for all internet checks.
	var portClient *host.PortCheckProviderClient
	if ip, err := host.ResolvePublicIP(d.HostDeps); err == nil {
		portClient = host.NewPortCheckProviderClient(d.HostDeps, ip)
	}

	// Resolve backup root and schedule file once.
	backupRoot := core.BackupRootDirForComposeFile(composeFile)
	schedFile, _ := core.CurrentAppDeps().ScheduleService.LoadScheduleFile()

	type instanceRow struct {
		s                 compose.InstanceSummary
		version           string
		internetReachable string
		backupScheduled   bool
		lastBackup        string
	}

	rowData := make([]instanceRow, 0, len(summaries))
	for _, s := range summaries {
		row := instanceRow{s: s, version: "-", internetReachable: "-", lastBackup: "-"}

		if s.Image != "" {
			if info, err := d.UpdateService.InspectImageVersionInfo(d.Context(), s.Image); err == nil && info.Version != "" {
				row.version = info.Version
			}
		}

		if s.Status != "running" {
			row.internetReachable = "stopped"
		} else if portClient != nil && s.ASAPort > 0 {
			target := host.PortCheckTarget{Protocol: host.ProtocolUDP, HostPort: s.ASAPort, ContainerPort: s.ASAPort}
			if isOpen, _, _, err := portClient.CheckExternalPortStatus(target); err == nil {
				row.internetReachable = core.YesNo(isOpen)
			}
		}

		row.backupScheduled = runtime.InstanceHasEnabledScheduleInFile(schedFile, s.Name)

		if backupRoot != "" {
			backupDir := filepath.Join(backupRoot, s.Name)
			if archives, err := backup.ListArchives(backupDir); err == nil && len(archives) > 0 {
				row.lastBackup = filepath.Base(archives[0])
			}
		}

		rowData = append(rowData, row)
	}

	if *jsonFlag {
		out := make([]core.InstanceSummaryJSON, 0, len(rowData))
		for _, r := range rowData {
			out = append(out, core.InstanceSummaryJSON{
				Name:              r.s.Name,
				Status:            r.s.Status,
				Map:               r.s.Map,
				Version:           r.version,
				Port:              r.s.ASAPort,
				InternetReachable: r.internetReachable,
				BackupScheduled:   r.backupScheduled,
				LastBackup:        r.lastBackup,
			})
		}
		return core.PrintJSONOutput(out)
	}

	rows := make([][]string, 0, len(rowData)+1)
	rows = append(rows, []string{"NAME", "STATUS", "MAP", "VERSION", "ASA_PORT", "INTERNET_REACHABLE", "BACKUP_SCHEDULED", "LAST_BACKUP"})
	for _, r := range rowData {
		rows = append(rows, []string{
			r.s.Name,
			r.s.Status,
			runtime.FallbackDash(r.s.Map),
			r.version,
			runtime.IntOrDash(r.s.ASAPort),
			r.internetReachable,
			core.YesNo(r.backupScheduled),
			r.lastBackup,
		})
	}
	core.PrintTable(rows)
	return nil
}

func formatBytes(n int64) string {
	const (
		KB = 1024
		MB = 1024 * KB
		GB = 1024 * MB
	)
	switch {
	case n >= GB:
		return fmt.Sprintf("%.1f GB", float64(n)/float64(GB))
	case n >= MB:
		return fmt.Sprintf("%.1f MB", float64(n)/float64(MB))
	case n >= KB:
		return fmt.Sprintf("%.1f KB", float64(n)/float64(KB))
	default:
		return fmt.Sprintf("%d B", n)
	}
}

func backupArchiveDate(archivePath string) string {
	name := backup.TrimArchiveSuffix(filepath.Base(archivePath))
	if m := backup.TimestampPattern.FindStringSubmatch(name); len(m) > 1 {
		ts := m[1]
		if len(ts) == 15 {
			return ts[:4] + "-" + ts[4:6] + "-" + ts[6:8] + " " + ts[9:11] + ":" + ts[11:13] + ":" + ts[13:15]
		}
	}
	info, err := os.Stat(archivePath)
	if err == nil {
		return info.ModTime().UTC().Format("2006-01-02 15:04:05")
	}
	return "-"
}

// RunListBackups lists backup archives for an instance or all instances.
func RunListBackups(d *core.AppDeps, args []string) error {
	fs := core.NewCommandFlagSet(
		"list backups",
		[]string{"list backups --instance <instance|all>"},
		"List backup archives for an instance or all instances.",
	)
	composeFileFlag := fs.String("compose-file", "", "Path to docker-compose.yaml (required for 'all').")
	jsonFlag := fs.Bool("json", false, "Output as JSON.")
	instanceFlag := fs.String("instance", "", "Instance name or 'all'.")
	if err := core.ParseNoPositionalArgs(fs, args); err != nil {
		return err
	}
	instance := *instanceFlag
	if instance == "" {
		return fmt.Errorf("Error: --instance is required.")
	}

	backupRoot := core.BackupRootDirForComposeFile(compose.ResolveComposeFile(*composeFileFlag))

	if instance != "all" {
		return listBackupsForInstance(d, backupRoot, instance, *jsonFlag)
	}

	_, config, err := compose.ResolveRequiredComposeConfig(d, *composeFileFlag)
	if err != nil {
		return err
	}
	services := config.ServiceNames()
	if len(services) == 0 {
		return errors.New("Error: compose config has no services.")
	}

	if *jsonFlag {
		out := make([]core.BackupEntryWithInstanceJSON, 0)
		for _, svc := range services {
			backupsDir := filepath.Join(backupRoot, svc)
			archives, _ := backup.ListArchives(backupsDir)
			for _, archive := range archives {
				info, err := os.Stat(archive)
				size := int64(0)
				if err == nil {
					size = info.Size()
				}
				out = append(out, core.BackupEntryWithInstanceJSON{Instance: svc, Name: filepath.Base(archive), SizeBytes: size, Date: backupArchiveDate(archive)})
			}
		}
		return core.PrintJSONOutput(out)
	}

	rows := make([][]string, 0)
	rows = append(rows, []string{"INSTANCE", "NAME", "SIZE", "DATE"})
	for _, svc := range services {
		backupsDir := filepath.Join(backupRoot, svc)
		archives, _ := backup.ListArchives(backupsDir)
		for _, archive := range archives {
			info, err := os.Stat(archive)
			size := int64(0)
			if err == nil {
				size = info.Size()
			}
			rows = append(rows, []string{svc, filepath.Base(archive), formatBytes(size), backupArchiveDate(archive)})
		}
	}
	core.PrintTable(rows)
	return nil
}

func listBackupsForInstance(d *core.AppDeps, backupRoot, instance string, asJSON bool) error {
	backupsDir := filepath.Join(backupRoot, instance)
	archives, err := backup.ListArchives(backupsDir)
	if err != nil {
		return fmt.Errorf("Error: backups directory not found for instance: %s", instance)
	}
	if len(archives) == 0 {
		return fmt.Errorf("Error: no backup archives found for instance: %s", instance)
	}

	if asJSON {
		out := make([]core.BackupEntryJSON, 0, len(archives))
		for _, archive := range archives {
			info, err := os.Stat(archive)
			size := int64(0)
			if err == nil {
				size = info.Size()
			}
			out = append(out, core.BackupEntryJSON{Name: filepath.Base(archive), SizeBytes: size, Date: backupArchiveDate(archive)})
		}
		return core.PrintJSONOutput(out)
	}

	rows := make([][]string, 0, len(archives)+1)
	rows = append(rows, []string{"NAME", "SIZE", "DATE"})
	for _, archive := range archives {
		info, err := os.Stat(archive)
		size := int64(0)
		if err == nil {
			size = info.Size()
		}
		rows = append(rows, []string{filepath.Base(archive), formatBytes(size), backupArchiveDate(archive)})
	}
	core.PrintTable(rows)
	return nil
}
