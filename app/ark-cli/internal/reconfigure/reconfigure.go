package reconfigure

import (
	"ark-cli/internal/compose"
	"ark-cli/internal/core"
	"errors"
	"flag"
	"fmt"
	"os"
	"regexp"
	"slices"
	"strings"

	"github.com/compose-spec/compose-go/v2/types"

	"pkg/ark/composeutil"
	"pkg/ark/runtime"
)

// modIDsPattern matches the MOD_IDS format the container accepts (runtime_utils.server_command).
var modIDsPattern = regexp.MustCompile(`^\d+(,\d+)*$`)

// clearableFlags take an explicit "" to clear their variable (an empty value
// means "not given" for every other flag).
var clearableFlags = []string{"server-password", "mods", "cluster-id"}

// clusterIDPattern matches the CLUSTER_ID format the container accepts.
var clusterIDPattern = regexp.MustCompile(`^[A-Za-z0-9_-]+$`)

type reconfigureCommandRequest struct {
	instance        string
	composeFileFlag string
	force           bool
	restartNow      bool

	tzSet             bool
	mapNameSet        bool
	sessionNameSet    bool
	adminPasswordSet  bool
	serverPasswordSet bool
	modsSet           bool
	clusterIDSet      bool
	asaPortSet        bool
	rconPortSet       bool
	rconHostIPSet     bool

	tz             string
	mapName        string
	sessionName    string
	adminPassword  string
	serverPassword string
	mods           string
	clusterID      string
	asaPort        int
	rconPort       int
	rconHostIP     string // "" publishes on all interfaces; see setRCONHostIP
}

func (r reconfigureCommandRequest) hasSettingUpdates() bool {
	return r.tzSet ||
		r.mapNameSet ||
		r.sessionNameSet ||
		r.adminPasswordSet ||
		r.serverPasswordSet ||
		r.modsSet ||
		r.clusterIDSet ||
		r.asaPortSet ||
		r.rconPortSet ||
		r.rconHostIPSet
}

func RunReconfigure(d *core.AppDeps, args []string) error {
	fs := core.NewCommandFlagSet(
		"reconfigure instance",
		[]string{"reconfigure instance --instance <instance> [options]"},
		"Update compose settings for a single instance.",
	)
	instanceFlag := fs.String("instance", "", "Instance name.")
	composeFileFlag := fs.String("compose-file", "", "Path to docker-compose.yaml (default: ./docker-compose.yaml or COMPOSE_FILE).")
	tz := fs.String("tz", "", "Set TZ environment value.")
	mapName := fs.String("map-name", "", "Set MAP_NAME environment value.")
	sessionName := fs.String("session-name", "", "Set SESSION_NAME environment value.")
	adminPassword := fs.String("admin-password", "", "Set SERVER_ADMIN_PASSWORD environment value.")
	serverPassword := fs.String("server-password", "", "Set SERVER_PASSWORD environment value (\"\" clears it: no join password).")
	mods := fs.String("mods", "", "Set MOD_IDS environment value (comma-separated CurseForge mod IDs; \"\" clears it).")
	clusterID := fs.String("cluster-id", "", "Set CLUSTER_ID environment value (instances with the same ID share transfers; \"\" clears it).")
	asaPort := fs.String("asa-port", "", "Set published ARK UDP port (host and container port are kept in sync).")
	rconPort := fs.String("rcon-port", "", "Set published RCON TCP host port (maps to 27020/tcp).")
	force := fs.Bool("force", false, "Allow reconfiguring while the instance is running (restart required for changes to take effect).")
	restartNow := fs.Bool("restart", false, "Restart the target instance immediately after updating compose.")

	if err := core.ParseNoPositionalArgs(fs, args); err != nil {
		return err
	}
	instance := *instanceFlag
	if instance == "" {
		return errors.New("Error: --instance is required.")
	}
	// These flags accept an explicit "" to clear the value, so record which were passed.
	var cleared []string
	fs.Visit(func(f *flag.Flag) {
		if f.Value.String() == "" && slices.Contains(clearableFlags, f.Name) {
			cleared = append(cleared, f.Name)
		}
	})

	req, err := buildReconfigureCommandRequest(
		instance,
		*composeFileFlag,
		*force,
		*restartNow,
		*tz,
		*mapName,
		*sessionName,
		*adminPassword,
		*serverPassword,
		*mods,
		*clusterID,
		*asaPort,
		*rconPort,
		cleared...,
	)
	if err != nil {
		return err
	}
	return runReconfigureCommand(req)
}

func buildReconfigureCommandRequest(
	instance,
	composeFileFlag string,
	force,
	restartNow bool,
	tz,
	mapName,
	sessionName,
	adminPassword,
	serverPassword,
	mods,
	clusterID,
	asaPort,
	rconPort string,
	cleared ...string, // clearableFlags passed with an empty value
) (reconfigureCommandRequest, error) {
	if err := runtime.ValidateComposeName(instance, "instance"); err != nil {
		return reconfigureCommandRequest{}, err
	}
	if instance == "all" {
		return reconfigureCommandRequest{}, errors.New("Error: reconfigure only supports one instance target.")
	}

	req := reconfigureCommandRequest{
		instance:          instance,
		composeFileFlag:   composeFileFlag,
		force:             force,
		restartNow:        restartNow,
		tzSet:             tz != "",
		mapNameSet:        mapName != "",
		sessionNameSet:    sessionName != "",
		adminPasswordSet:  adminPassword != "",
		serverPasswordSet: serverPassword != "" || slices.Contains(cleared, "server-password"),
		modsSet:           mods != "" || slices.Contains(cleared, "mods"),
		clusterIDSet:      clusterID != "" || slices.Contains(cleared, "cluster-id"),
		asaPortSet:        asaPort != "",
		rconPortSet:       rconPort != "",
		tz:                tz,
		mapName:           mapName,
		sessionName:       sessionName,
		adminPassword:     adminPassword,
		serverPassword:    serverPassword,
		mods:              strings.Join(strings.Fields(mods), ""),
		clusterID:         clusterID,
	}

	if req.tzSet {
		if err := runtime.ValidateYAMLScalar(req.tz, "time zone"); err != nil {
			return reconfigureCommandRequest{}, err
		}
	}
	if req.mapNameSet {
		if err := runtime.ValidateYAMLScalar(req.mapName, "map name"); err != nil {
			return reconfigureCommandRequest{}, err
		}
		if runtime.MapNameMissingWP(req.mapName) {
			core.Warnf("\nWARNING: map name %q does not end in _WP.\nAll current ARK: Survival Ascended maps require the _WP suffix.\nThe server may fail to start or hang indefinitely.\n", req.mapName)
		}
	}
	if req.sessionNameSet {
		if err := runtime.ValidateYAMLScalar(req.sessionName, "session name"); err != nil {
			return reconfigureCommandRequest{}, err
		}
	}
	if req.adminPasswordSet {
		if err := runtime.ValidateYAMLScalar(req.adminPassword, "admin password"); err != nil {
			return reconfigureCommandRequest{}, err
		}
		if err := runtime.ValidatePassword(req.adminPassword, "admin password"); err != nil {
			return reconfigureCommandRequest{}, err
		}
	}
	if req.serverPasswordSet && req.serverPassword != "" {
		if err := runtime.ValidateYAMLScalar(req.serverPassword, "server password"); err != nil {
			return reconfigureCommandRequest{}, err
		}
		if err := runtime.ValidatePassword(req.serverPassword, "server password"); err != nil {
			return reconfigureCommandRequest{}, err
		}
	}
	if req.modsSet && req.mods != "" && !modIDsPattern.MatchString(req.mods) {
		return reconfigureCommandRequest{}, fmt.Errorf("Error: --mods must be comma-separated CurseForge mod IDs, got %q.", mods)
	}
	if req.clusterIDSet && req.clusterID != "" && !clusterIDPattern.MatchString(req.clusterID) {
		return reconfigureCommandRequest{}, fmt.Errorf("Error: --cluster-id may contain only letters, digits, \"_\" and \"-\", got %q.", clusterID)
	}
	if req.asaPortSet {
		parsed, err := runtime.ParsePort(asaPort, "ASA port")
		if err != nil {
			return reconfigureCommandRequest{}, err
		}
		req.asaPort = parsed
	}
	if req.rconPortSet {
		parsed, err := runtime.ParsePort(rconPort, "RCON port")
		if err != nil {
			return reconfigureCommandRequest{}, err
		}
		req.rconPort = parsed
	}
	if !req.hasSettingUpdates() {
		return reconfigureCommandRequest{}, errors.New("Error: provide at least one setting flag to reconfigure.")
	}
	return req, nil
}

func runReconfigureCommand(req reconfigureCommandRequest) error {
	composeFile := compose.ResolveComposeFile(req.composeFileFlag)
	if err := compose.RequireComposeFile(composeFile); err != nil {
		return err
	}
	if err := core.CurrentAppDeps().RuntimeHelper.EnsureDockerGroup(os.Stderr); err != nil {
		return err
	}
	running, err := core.CurrentAppDeps().RuntimeHelper.ServiceRunningStatus(composeFile, req.instance)
	if err != nil {
		return err
	}
	if running && !req.force {
		return errors.New("Error: cannot reconfigure while service is running. Use --force to override.")
	}
	wasRunning := running

	composeProject, err := core.CurrentAppDeps().ComposeService.LoadProject(core.CurrentAppDeps().Context(), composeFile)
	if err != nil {
		return err
	}
	project := composeProject.Project
	svc, ok := project.Services[req.instance]
	if !ok {
		return fmt.Errorf("Error: service not found in compose file: %s", req.instance)
	}

	if err := guardReconfigureMapName(svc, req); err != nil {
		return err
	}

	changed := applyReconfigureEnvUpdates(project, req.instance, req)
	portChanged, err := applyReconfigurePortUpdates(project, req)
	if err != nil {
		return err
	}
	changed = changed || portChanged

	if changed {
		if err := core.CurrentAppDeps().ComposeService.SaveProject(core.CurrentAppDeps().Context(), composeFile, composeProject); err != nil {
			return err
		}
		core.Infof("Updated compose settings for instance %s in %s\n", req.instance, composeFile)
	} else {
		core.Infof("No compose setting changes were necessary for instance %s\n", req.instance)
	}
	return handleReconfigureRestart(composeFile, req, changed, wasRunning)
}

func applyReconfigureEnvUpdates(project *types.Project, serviceName string, req reconfigureCommandRequest) bool {
	svc := project.Services[serviceName]
	env := composeutil.EnvAsMap(svc.Environment)
	changed := false

	changed = setEnvIfRequested(env, "TZ", req.tzSet, req.tz) || changed
	changed = setEnvIfRequested(env, "MAP_NAME", req.mapNameSet, req.mapName) || changed
	changed = setEnvIfRequested(env, "SESSION_NAME", req.sessionNameSet, req.sessionName) || changed
	changed = setEnvIfRequested(env, "SERVER_ADMIN_PASSWORD", req.adminPasswordSet, req.adminPassword) || changed
	changed = setEnvIfRequested(env, "SERVER_PASSWORD", req.serverPasswordSet, req.serverPassword) || changed
	changed = setEnvIfRequested(env, "MOD_IDS", req.modsSet, req.mods) || changed
	changed = setEnvIfRequested(env, "CLUSTER_ID", req.clusterIDSet, req.clusterID) || changed

	if changed {
		composeutil.MutateService(project, serviceName, func(s *types.ServiceConfig) {
			if s.Environment == nil {
				s.Environment = types.MappingWithEquals{}
			}
			for k, v := range env {
				composeutil.SetEnv(s.Environment, k, v)
			}
		})
	}
	return changed
}

func setEnvIfRequested(env map[string]string, key string, enabled bool, value string) bool {
	if !enabled {
		return false
	}
	before, ok := env[key]
	env[key] = value
	return !ok || before != value
}

func handleReconfigureRestart(composeFile string, req reconfigureCommandRequest, changed bool, wasRunning bool) error {
	if req.restartNow {
		if !wasRunning {
			core.Infof(
				"Reconfigure applied for %s. Server was not started because it was not previously running.\n",
				req.instance,
			)
			return nil
		}
		d := core.CurrentAppDeps()
		err := compose.WithInstanceLocks(d, composeFile, []string{req.instance}, func() error {
			if err := compose.DoRestartStopInstance(d, runtime.StopCommandRequest{
				Instance:        req.instance,
				ComposeFileFlag: composeFile,
				Timeout:         -1, // default grace so the server saves
			}); err != nil {
				return err
			}
			return compose.DoRestartStartInstance(d, runtime.StartCommandRequest{
				Instance:        req.instance,
				ComposeFileFlag: composeFile,
				WaitTimeout:     compose.DefaultStartReadyWaitTimeout,
			})
		})
		if err != nil {
			return err
		}
		core.Infof("Restarted instance %s.\n", req.instance)
		return nil
	}
	if !changed {
		return nil
	}
	core.Infof("Restart required for settings to take effect: %s\n", req.instance)
	return nil
}

func guardReconfigureMapName(service types.ServiceConfig, req reconfigureCommandRequest) error {
	if !req.mapNameSet {
		return nil
	}
	existing, _ := composeutil.EnvValue(service.Environment, "MAP_NAME")
	if existing == "" || existing == req.mapName {
		return nil
	}
	return fmt.Errorf(
		"Error: cannot change MAP_NAME on an existing instance (currently %q). Create a new instance to use a different map.",
		existing,
	)
}
