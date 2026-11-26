package show

import (
	"ark-cli/internal/compose"
	"ark-cli/internal/core"
	"ark-cli/internal/ini"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/compose-spec/compose-go/v2/types"
	"github.com/goccy/go-yaml"
	"pkg/ark"
	backup "pkg/ark/backup"
	"pkg/ark/host"
	"pkg/ark/runtime"
	"pkg/ark/schedule"
)

// RunShowInstance shows full configuration and status for an instance.
func RunShowInstance(d *core.AppDeps, args []string) error {
	fs := core.NewCommandFlagSet(
		"show instance",
		[]string{"show instance --instance <instance> [options]"},
		"Show full configuration and status for an instance.",
	)
	composeFileFlag := fs.String("compose-file", "", "Path to docker-compose.yaml (default: ./docker-compose.yaml or COMPOSE_FILE).")
	jsonFlag := fs.Bool("json", false, "Output as JSON.")
	instanceFlag := fs.String("instance", "", "Instance name.")
	if err := core.ParseNoPositionalArgs(fs, args); err != nil {
		return err
	}
	instance := *instanceFlag
	if instance == "" {
		return fmt.Errorf("Error: --instance is required.")
	}

	composeFile, config, err := compose.ResolveRequiredServiceConfig(d, *composeFileFlag, instance, compose.ResolveRequiredComposeConfig)
	if err != nil {
		return err
	}
	service := config.Project.Services[instance]
	summary := d.RuntimeHelper.BuildInstanceSummary(composeFile, instance, service)

	// Version from image label.
	version := "-"
	if summary.Image != "" {
		if info, err := d.UpdateService.InspectImageVersionInfo(d.Context(), summary.Image); err == nil && info.Version != "" {
			version = info.Version
		}
	}

	// Volumes.
	volPaths, _ := instanceVolumePaths(d, composeFile, config, service)

	// Port status: only probe when running.
	localhostStatus := "-"
	internetStatus := "-"
	if summary.Status != "running" {
		localhostStatus = "stopped"
		internetStatus = "stopped"
	} else if summary.ASAPort > 0 {
		target := host.PortCheckTarget{Protocol: host.ProtocolUDP, HostPort: summary.ASAPort, ContainerPort: summary.ASAPort}
		_, _, raw, _ := host.CheckPortLocalhost(target, "")
		if raw != "" {
			localhostStatus = raw
		}
		if ip, err := host.ResolvePublicIP(d.HostDeps); err == nil {
			client := host.NewPortCheckProviderClient(d.HostDeps, ip)
			if isOpen, _, _, err := client.CheckExternalPortStatus(target); err == nil {
				internetStatus = core.YesNo(isOpen)
			}
		}
	}

	// Schedules for this instance.
	var instanceSchedules []schedule.BackupSchedule
	if schedFile, err := d.ScheduleService.LoadScheduleFile(d.Context(), d.ComposeService, composeFile); err == nil {
		for _, s := range schedFile.Schedules {
			if s.Instance == instance || s.Instance == "all" {
				instanceSchedules = append(instanceSchedules, s)
			}
		}
	}

	// Backups.
	var backupDir string
	var backups []string
	backupRoot := core.BackupRootDirForComposeFile(composeFile)
	backupDir = filepath.Join(backupRoot, instance)
	backups, _ = backup.ListArchives(backupDir)

	// INI content via docker exec (running) or docker run --rm (stopped).
	isRunning := summary.Status == "running"
	gameIni := captureIniContent(d, composeFile, instance, isRunning, ini.IniFileSpec{Key: "game", Filename: "Game.ini"})
	gusIni := captureIniContent(d, composeFile, instance, isRunning, ini.IniFileSpec{Key: "gameusersettings", Filename: "GameUserSettings.ini"})

	if *jsonFlag {
		j := core.InstanceInspectJSON{
			Name:                summary.Name,
			Status:              summary.Status,
			Image:               summary.Image,
			Version:             version,
			Map:                 summary.Map,
			Session:             summary.Session,
			ASAPort:             summary.ASAPort,
			ComposeFile:         composeFile,
			PortStatus:          core.PortStatusJSON{Port: summary.ASAPort, Localhost: localhostStatus, Internet: internetStatus},
			BackupDir:           backupDir,
			GameIni:             gameIni,
			GameUserSettingsIni: gusIni,
		}
		for _, v := range volPaths {
			j.Volumes = append(j.Volumes, core.VolumeInspectJSON{
				Volume:          v.declaredName,
				DockerVolume:    v.dockerName,
				ContainerTarget: v.targetPath,
				HostPath:        v.hostPath,
			})
		}
		for _, s := range instanceSchedules {
			j.Schedules = append(j.Schedules, core.ScheduleEntryJSON{
				Name:     s.Name,
				Enabled:  s.Enabled,
				Type:     s.Type,
				Instance: s.Instance,
				Cron:     s.Cron,
			})
		}
		for _, archive := range backups {
			info, statErr := os.Stat(archive)
			size := int64(0)
			if statErr == nil {
				size = info.Size()
			}
			j.Backups = append(j.Backups, core.BackupEntryJSON{
				Name:      filepath.Base(archive),
				SizeBytes: size,
				Date:      backupArchiveDate(archive),
			})
		}
		return core.PrintJSONOutput(j)
	}

	// Text output — sectioned.
	printSection("INSTANCE")
	core.Infof("name:     %s\n", summary.Name)
	core.Infof("status:   %s\n", summary.Status)
	core.Infof("image:    %s\n", runtime.FallbackDash(summary.Image))
	core.Infof("version:  %s\n", version)
	core.Infof("map:      %s\n", runtime.FallbackDash(summary.Map))
	core.Infof("session:  %s\n", runtime.FallbackDash(summary.Session))
	core.Infof("port:     %s\n", runtime.IntOrDash(summary.ASAPort))

	printSection("COMPOSE")
	core.Infof("compose_file: %s\n", composeFile)
	if serviceYAML, err := yaml.Marshal(service); err == nil {
		core.Infof("%s", string(serviceYAML))
	}

	printSection("VOLUMES")
	if len(volPaths) == 0 {
		core.Infoln("(none)")
	} else {
		rows := [][]string{{"VOLUME", "DOCKER_VOLUME", "CONTAINER_TARGET", "HOST_PATH"}}
		for _, v := range volPaths {
			rows = append(rows, []string{v.declaredName, v.dockerName, v.targetPath, v.hostPath})
		}
		core.PrintTable(rows)
	}

	printSection("PORT STATUS")
	if summary.ASAPort <= 0 {
		core.Infoln("(no port configured)")
	} else {
		core.PrintTable([][]string{
			{"LABEL", "PORT", "PROTO", "LOCALHOST", "INTERNET"},
			{"ASA", runtime.IntOrDash(summary.ASAPort), "udp", localhostStatus, internetStatus},
		})
	}

	printSection("SCHEDULES")
	if len(instanceSchedules) == 0 {
		core.Infoln("(none)")
	} else {
		rows := [][]string{{"NAME", "ENABLED", "CRON"}}
		for _, s := range instanceSchedules {
			rows = append(rows, []string{s.Name, core.YesNo(s.Enabled), s.Cron})
		}
		core.PrintTable(rows)
	}

	printSection("BACKUPS")
	if backupDir != "" {
		core.Infof("backup_dir: %s\n", backupDir)
	}
	if len(backups) == 0 {
		core.Infoln("(none)")
	} else {
		rows := [][]string{{"NAME", "SIZE", "DATE"}}
		for _, archive := range backups {
			info, statErr := os.Stat(archive)
			size := int64(0)
			if statErr == nil {
				size = info.Size()
			}
			rows = append(rows, []string{filepath.Base(archive), formatBytes(size), backupArchiveDate(archive)})
		}
		core.PrintTable(rows)
	}

	printSection("Game.ini")
	core.Infof("%s\n", gameIni)

	printSection("GameUserSettings.ini")
	core.Infof("%s\n", gusIni)

	return nil
}

func captureIniContent(d *core.AppDeps, composeFile, instance string, running bool, fileSpec ini.IniFileSpec) string {
	content, err := ini.ReadIni(d, composeFile, instance, running, fileSpec)
	if err != nil {
		return err.Error()
	}
	content = strings.TrimSpace(content)
	if content == "" {
		return "(empty)"
	}
	return content
}

func printSection(title string) {
	core.Infof("\n=== %s ===\n", title)
}

// RunShowBackup shows backup archive metadata.
func RunShowBackup(d *core.AppDeps, args []string) error {
	fs := core.NewCommandFlagSet(
		"show backup",
		[]string{"show backup --instance <instance> --backup <backup|latest> [options]"},
		"Show backup archive metadata.",
	)
	jsonFlag := fs.Bool("json", false, "Output as JSON.")
	instanceFlag := fs.String("instance", "", "Instance name.")
	backupFlag := fs.String("backup", "", "Backup name or 'latest'.")
	if err := core.ParseNoPositionalArgs(fs, args); err != nil {
		return err
	}
	instance := *instanceFlag
	backupName := *backupFlag
	if instance == "" || backupName == "" {
		return fmt.Errorf("Error: --instance and --backup are required.")
	}

	backupRoot := core.BackupRootDir()
	backupsDir := filepath.Join(backupRoot, instance)
	archive, err := backup.ResolveArchive(backupsDir, backupName)
	if err != nil {
		return err
	}
	info, err := os.Stat(archive)
	if err != nil {
		return err
	}
	name := filepath.Base(archive)
	format := backup.DetectArchiveFormat(name)
	if format == "" {
		format = "unknown"
	}
	timestampHint := ""
	if m := backup.TimestampPattern.FindStringSubmatch(backup.TrimArchiveSuffix(name)); len(m) > 1 {
		timestampHint = m[1]
	}

	var metadata backup.BackupMetadata
	if d.BackupService != nil {
		metadata, _ = d.BackupService.ReadArchiveMetadata(d.Context(), archive)
	}

	if *jsonFlag {
		return core.PrintJSONOutput(core.BackupDetailJSON{
			Instance:        instance,
			Name:            name,
			Format:          format,
			Path:            archive,
			SizeBytes:       info.Size(),
			ModifiedUTC:     info.ModTime().UTC().Format("2006-01-02T15:04:05Z"),
			TimestampHint:   timestampHint,
			ServerVersion:   metadata.ServerVersion,
			BuildID:         metadata.BuildID,
			InstanceName:    metadata.InstanceName,
			MapName:         metadata.MapName,
			BackupTimestamp: metadata.BackupTimestamp,
		})
	}

	tsHintDisplay := runtime.FallbackDash(timestampHint)
	core.Infof("instance: %s\n", instance)
	core.Infof("name: %s\n", name)
	core.Infof("format: %s\n", format)
	core.Infof("path: %s\n", archive)
	core.Infof("size_bytes: %d\n", info.Size())
	core.Infof("modified_utc: %s\n", info.ModTime().UTC().Format("2006-01-02T15:04:05Z"))
	core.Infof("timestamp_hint: %s\n", tsHintDisplay)
	core.Infof("server_version: %s\n", runtime.FallbackDash(metadata.ServerVersion))
	core.Infof("build_id: %s\n", runtime.FallbackDash(metadata.BuildID))
	core.Infof("instance_name: %s\n", runtime.FallbackDash(metadata.InstanceName))
	core.Infof("map_name: %s\n", runtime.FallbackDash(metadata.MapName))
	core.Infof("backup_timestamp: %s\n", runtime.FallbackDash(metadata.BackupTimestamp))
	return nil
}

// RunShowCompose shows compose configuration summary.
func RunShowCompose(d *core.AppDeps, args []string) error {
	fs := core.NewCommandFlagSet(
		"show compose",
		[]string{"show compose [options]"},
		"Show compose configuration summary.",
	)
	composeFileFlag := fs.String("compose-file", "", "Path to docker-compose.yaml (default: ./docker-compose.yaml or COMPOSE_FILE).")
	if err := core.ParseNoPositionalArgs(fs, args); err != nil {
		return err
	}

	composeFile, config, err := compose.ResolveRequiredComposeConfig(d, *composeFileFlag)
	if err != nil {
		return err
	}
	serviceNames := config.ServiceNames()
	volumesCount := len(config.Project.Volumes)
	core.Infof("compose_file: %s\n", composeFile)
	core.Infof("services: %d\n", len(serviceNames))
	core.Infof("volumes: %d\n", volumesCount)
	core.Infof("service_names: %s\n", strings.Join(serviceNames, ","))
	return nil
}

type VolumePathInfo struct {
	declaredName string
	dockerName   string
	targetPath   string
	hostPath     string
}

func serviceNamedVolume(vol types.ServiceVolumeConfig, declared types.Volumes) (string, string, bool) {
	source := vol.Source
	target := vol.Target
	kind := vol.Type
	if source == "" || target == "" {
		return "", "", false
	}
	if kind != "" && kind != "volume" {
		return "", "", false
	}
	if _, ok := declared[source]; !ok {
		return "", "", false
	}
	return source, target, true
}

func volumeInspectCandidates(composeFile, volume string) []string {
	projectVolume := fmt.Sprintf("%s_%s", runtime.ComposeProjectName(composeFile), volume)
	if projectVolume == volume {
		return []string{volume}
	}
	return []string{projectVolume, volume}
}

func resolveVolumePathInfo(d *core.AppDeps, composeFile, volume, target string) (VolumePathInfo, error) {
	for _, candidate := range volumeInspectCandidates(composeFile, volume) {
		if hostPath, ok := d.Engine.VolumeMountpoint(d.Context(), candidate); ok {
			return VolumePathInfo{
				declaredName: volume,
				dockerName:   candidate,
				targetPath:   target,
				hostPath:     hostPath,
			}, nil
		}
	}
	return VolumePathInfo{}, fmt.Errorf("Error: could not resolve volume mountpoint for %s", volume)
}

func instanceVolumePaths(d *core.AppDeps,
	composeFile string,
	project *ark.ComposeProject,
	service types.ServiceConfig,
) ([]VolumePathInfo, error) {
	declared := project.Project.Volumes
	paths := make([]VolumePathInfo, 0, len(service.Volumes))
	for _, vol := range service.Volumes {
		volume, target, ok := serviceNamedVolume(vol, declared)
		if !ok {
			continue
		}
		info, err := resolveVolumePathInfo(d, composeFile, volume, target)
		if err != nil {
			return nil, err
		}
		paths = append(paths, info)
	}
	if len(paths) == 0 {
		return nil, errors.New("Error: no named volumes are attached to this instance.")
	}
	return paths, nil
}

// RunShowVolumePaths shows volume mount paths for an instance.
func RunShowVolumePaths(d *core.AppDeps, args []string) error {
	fs := core.NewCommandFlagSet(
		"show volume-paths",
		[]string{"show volume-paths --instance <instance> [options]"},
		"Show volume mount paths for an instance.",
	)
	composeFileFlag := fs.String("compose-file", "", "Path to docker-compose.yaml (default: ./docker-compose.yaml or COMPOSE_FILE).")
	instanceFlag := fs.String("instance", "", "Instance name.")
	if err := core.ParseNoPositionalArgs(fs, args); err != nil {
		return err
	}
	instance := *instanceFlag
	if instance == "" {
		return fmt.Errorf("Error: --instance is required.")
	}

	composeFile, config, err := compose.ResolveRequiredServiceConfig(d, *composeFileFlag, instance, compose.ResolveRequiredComposeConfig)
	if err != nil {
		return err
	}
	paths, err := instanceVolumePaths(d, composeFile, config, config.Project.Services[instance])
	if err != nil {
		return err
	}

	core.Infof("instance:     %s\n", instance)
	core.Infof("compose_file: %s\n", composeFile)
	core.Infof("\n")

	rows := make([][]string, 0, len(paths)+1)
	rows = append(rows, []string{"VOLUME", "DOCKER_VOLUME", "CONTAINER_TARGET", "HOST_PATH"})
	for _, p := range paths {
		rows = append(rows, []string{p.declaredName, p.dockerName, p.targetPath, p.hostPath})
	}
	core.PrintTable(rows)

	core.Infof("\nNotice: Directly altering files in a container volume from outside the container can result in corrupted file permissions. User assumes responsibility when accessing these paths manually.\n")
	return nil
}
