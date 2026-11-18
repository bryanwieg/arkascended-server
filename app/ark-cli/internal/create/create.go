package create

import (
	"ark-cli/internal/compose"
	"ark-cli/internal/core"
	"bufio"
	"errors"
	"fmt"
	"os"
	"strconv"

	pkgbackup "pkg/ark/backup"
	"pkg/ark/runtime"
)

const (
	defaultTZ           = "America/New_York"
	defaultMap          = "TheIsland_WP"
	defaultASAPort      = 27015
	defaultRCONHostPort = 27020
	defaultImage        = "ark-server:latest"
)

type CreateSettings struct {
	instance       string
	mapName        string
	sessionName    string
	adminPassword  string
	serverPassword string
	asaPort        int
	rconHostPort   int
	tz             string
	image          string
}

type createParams struct {
	mapName          string
	adminPassword    string
	serverPassword   string
	tz               string
	asaPort          int
	interactive      bool
	noServerPassword bool
}

func resolveInteractiveInputs(reader *bufio.Reader, mapName, adminPw, serverPw, tz string, asaPort int, noServerPassword bool) (string, string, string, string, int, error) {
	var err error
	if mapName, err = promptNonEmpty(reader, "Map name", defaultMap, mapName); err != nil {
		return "", "", "", "", 0, err
	}
	if adminPw, err = promptRequired(reader, "Server admin password"); err != nil {
		return "", "", "", "", 0, err
	}
	if !noServerPassword {
		if serverPw, err = promptRequired(reader, "Server password"); err != nil {
			return "", "", "", "", 0, err
		}
	}
	if tz, err = promptNonEmpty(reader, "Time zone", defaultTZ, tz); err != nil {
		return "", "", "", "", 0, err
	}
	asaDefault := "auto"
	if asaPort != 0 {
		asaDefault = strconv.Itoa(asaPort)
	}
	value, err := promptOptional(reader, fmt.Sprintf("ASA port [%s]", asaDefault))
	if err != nil {
		return "", "", "", "", 0, err
	}
	if value != "" {
		parsed, err := runtime.ParsePort(value, "ASA port")
		if err != nil {
			return "", "", "", "", 0, err
		}
		asaPort = parsed
	} else {
		asaPort = 0
	}
	return mapName, adminPw, serverPw, tz, asaPort, nil
}

func resolveNonInteractiveInputs(mapName, adminPw, serverPw, tz string, asaPort int, noServerPassword bool) (string, string, string, string, int, error) {
	if adminPw == "" {
		return "", "", "", "", 0, errors.New("Error: --admin-password is required (or use --interactive).")
	}
	if !noServerPassword && serverPw == "" {
		return "", "", "", "", 0, errors.New("Error: --server-password is required (or use --interactive, or --no-server-password).")
	}
	if mapName == "" {
		mapName = defaultMap
	}
	if tz == "" {
		tz = defaultTZ
	}
	if asaPort != 0 {
		parsed, err := runtime.ParsePort(strconv.Itoa(asaPort), "ASA port")
		if err != nil {
			return "", "", "", "", 0, err
		}
		asaPort = parsed
	}
	return mapName, adminPw, serverPw, tz, asaPort, nil
}

func resolveCreateInputs(reader *bufio.Reader, params createParams) (string, string, string, string, int, error) {
	if params.interactive {
		return resolveInteractiveInputs(reader, params.mapName, params.adminPassword, params.serverPassword, params.tz, params.asaPort, params.noServerPassword)
	}
	return resolveNonInteractiveInputs(params.mapName, params.adminPassword, params.serverPassword, params.tz, params.asaPort, params.noServerPassword)
}

func assignCreatePorts(composeText string, asaPort int) (int, int) {
	usedUDP := runtime.FindUsedPorts(composeText, "udp")
	usedTCP := runtime.FindUsedPorts(composeText, "tcp")

	if asaPort == 0 {
		asaPort = runtime.PickNextPort(usedUDP, defaultASAPort)
	}
	rconHostPort := runtime.PickNextPort(usedTCP, defaultRCONHostPort)
	return asaPort, rconHostPort
}

func validateCreateSettingsFields(settings CreateSettings) error {
	if err := runtime.ValidateYAMLScalar(settings.mapName, "map name"); err != nil {
		return err
	}
	if err := runtime.ValidateYAMLScalar(settings.sessionName, "session name"); err != nil {
		return err
	}
	if err := runtime.ValidateYAMLScalar(settings.adminPassword, "admin password"); err != nil {
		return err
	}
	if err := runtime.ValidatePassword(settings.adminPassword, "admin password"); err != nil {
		return err
	}
	if err := runtime.ValidateYAMLScalar(settings.serverPassword, "server password"); err != nil {
		return err
	}
	if settings.serverPassword != "" {
		if err := runtime.ValidatePassword(settings.serverPassword, "server password"); err != nil {
			return err
		}
	}
	if err := runtime.ValidateYAMLScalar(settings.tz, "time zone"); err != nil {
		return err
	}
	if err := runtime.ValidateYAMLScalar(settings.image, "image"); err != nil {
		return err
	}
	return nil
}

func readExistingComposeText(composeFile string) (string, error) {
	data, err := os.ReadFile(composeFile)
	if err == nil {
		return string(data), nil
	}
	if errors.Is(err, os.ErrNotExist) {
		return "", nil
	}
	return "", err
}

func serviceExistsInComposeText(composeText, name string) bool {
	return runtime.EnsureServiceAbsent(composeText, name) != nil
}

func mergeCreateServiceIntoCompose(composeText, rendered string, settings CreateSettings) (string, error) {
	if composeText == "" {
		return rendered, nil
	}
	if err := runtime.EnsureServiceAbsent(composeText, settings.instance); err != nil {
		return "", err
	}
	serviceBlock, err := runtime.ExtractServiceBlock(rendered)
	if err != nil {
		return "", err
	}
	updated := runtime.InsertService(composeText, serviceBlock)
	updated = runtime.EnsureVolumeEntry(updated, fmt.Sprintf("%s-saved", settings.instance))
	updated = runtime.EnsureVolumeEntry(updated, "ark-cluster-1")
	return updated, nil
}

func RunCreate(d *core.AppDeps, args []string) error {
	fs := core.NewCommandFlagSet(
		"create",
		[]string{"create instance --name <name> [options]"},
		"Create or extend a docker-compose.yaml with an ARK instance service.",
	)

	nameFlag := fs.String("name", "", "Instance name.")
	mapName := fs.String("map-name", "", "Map name (default: "+defaultMap+")")
	serverPassword := fs.String("server-password", "", "Server password")
	adminPassword := fs.String("admin-password", "", "Server admin password")
	asaPort := fs.Int("asa-port", 0, "Host UDP port to expose (default: auto)")
	tz := fs.String("tz", "", "Time zone (default: America/New_York)")
	interactive := fs.Bool("interactive", false, "Prompt for values instead of requiring flags")
	noServerPassword := fs.Bool("no-server-password", false, "Create instance without a server password (sets SERVER_PASSWORD to empty)")
	composeFileFlag := fs.String("compose-file", "", "Path to docker-compose.yaml (default: ./docker-compose.yaml or COMPOSE_FILE).")

	if err := core.ParseNoPositionalArgs(fs, args); err != nil {
		return err
	}
	instance := *nameFlag
	if instance == "" {
		return errors.New("Error: --name is required.")
	}
	if err := runtime.ValidateComposeName(instance, "instance name"); err != nil {
		return err
	}
	if instance == pkgbackup.ClusterTarget {
		return pkgbackup.ErrClusterNameReserved
	}

	composeFile := compose.ResolveComposeFile(*composeFileFlag)

	composeText, err := readExistingComposeText(composeFile)
	if err != nil {
		return err
	}

	if composeText != "" && serviceExistsInComposeText(composeText, instance) {
		return fmt.Errorf("Error: instance '%s' already exists. Use 'remove instance %s' first.", instance, instance)
	}

	if *noServerPassword && *serverPassword != "" {
		return fmt.Errorf("Error: --server-password and --no-server-password are mutually exclusive.")
	}
	settings, err := buildCreateSettings(instance, createParams{
		mapName:          *mapName,
		adminPassword:    *adminPassword,
		serverPassword:   *serverPassword,
		tz:               *tz,
		asaPort:          *asaPort,
		interactive:      *interactive,
		noServerPassword: *noServerPassword,
	}, composeText, defaultImage)
	if err != nil {
		return err
	}

	composeSettings := compose.CreateSettings{
		Instance:       settings.instance,
		MapName:        settings.mapName,
		SessionName:    settings.sessionName,
		AdminPassword:  settings.adminPassword,
		ServerPassword: settings.serverPassword,
		ASAPort:        settings.asaPort,
		RCONHostPort:   settings.rconHostPort,
		TZ:             settings.tz,
		Image:          settings.image,
	}
	rendered := compose.RenderTemplate(composeSettings)
	updated, err := mergeCreateServiceIntoCompose(composeText, rendered, settings)
	if err != nil {
		return err
	}

	if err := os.WriteFile(composeFile, []byte(updated), 0o644); err != nil {
		return err
	}
	core.Infof("Wrote compose file: %s\n", composeFile)
	return nil
}

func buildCreateSettings(instance string, params createParams, composeText, image string) (CreateSettings, error) {
	mapName, adminPassword, serverPassword, tz, asaPort, err := resolveCreateInputs(
		bufio.NewReader(os.Stdin),
		params,
	)
	if err != nil {
		return CreateSettings{}, err
	}
	asaPort, rconHostPort := assignCreatePorts(composeText, asaPort)
	if image == "" {
		image = defaultImage
	}
	settings := CreateSettings{
		instance:       instance,
		mapName:        mapName,
		sessionName:    instance,
		adminPassword:  adminPassword,
		serverPassword: serverPassword,
		asaPort:        asaPort,
		rconHostPort:   rconHostPort,
		tz:             tz,
		image:          image,
	}
	if err := validateCreateSettingsFields(settings); err != nil {
		return CreateSettings{}, err
	}
	if runtime.MapNameMissingWP(settings.mapName) {
		core.Warnf("\nWARNING: map name %q does not end in _WP.\nAll current ARK: Survival Ascended maps require the _WP suffix.\nThe server may fail to start or hang indefinitely.\n", settings.mapName)
	}
	return settings, nil
}
