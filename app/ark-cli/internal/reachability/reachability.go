package reachability

import (
	"errors"
	"fmt"
	"net"
	"os"
	"strconv"
	"strings"

	"ark-cli/internal/compose"
	"ark-cli/internal/core"

	"github.com/compose-spec/compose-go/v2/types"

	"pkg/ark/host"
	"pkg/ark/runtime"
)

func RunCheckReachability(d *core.AppDeps, args []string) error {
	fs := core.NewCommandFlagSet(
		"check_reachability",
		[]string{
			"check reachability --port <num> [options]",
			"check reachability --instance <name> [options]",
		},
		"Check port reachability from the internet and/or localhost.",
	)
	fromFlag := fs.String("from", "both", "Probe vantage point: local, remote, <addr>, or both.")
	listenFlag := fs.Bool("listen", false, "Open temporary listener(s) during the check to test forwarding without a running server.")
	composeFileFlag := fs.String("compose-file", "", "Path to docker-compose.yaml (default: ./docker-compose.yaml or COMPOSE_FILE).")
	protocolFlag := fs.String("protocol", "both", "Protocol to probe: tcp, udp, or both (port mode only).")
	portsFlag := fs.String("ports", "all", "Instance ports to check: asa, rcon, or all (instance mode only).")
	portFlag := fs.Int("port", 0, "Port number to check.")
	instanceFlag := fs.String("instance", "", "Instance name to check its ports.")

	if err := core.ParseNoPositionalArgs(fs, args); err != nil {
		return err
	}
	if *portFlag > 0 && *instanceFlag != "" {
		return errors.New("Error: --port and --instance are mutually exclusive.")
	}
	if *portFlag > 0 {
		return runPortMode(d, strconv.Itoa(*portFlag), *fromFlag, *listenFlag, *protocolFlag, *portsFlag)
	}
	if *instanceFlag != "" {
		return runInstanceMode(d, *instanceFlag, *fromFlag, *listenFlag, *composeFileFlag, *portsFlag)
	}
	return errors.New("Error: specify --port <num> or --instance <name>.")
}

func runPortMode(d *core.AppDeps, portValue, from string, listen bool, protocol, portsFlag string) error {
	if portsFlag != "all" {
		return errors.New("Error: --ports is only supported with instance mode.")
	}

	side, hostAddr := parseFromFlag(from)

	if listen && side == host.CheckSideInternet {
		return errors.New("Error: --listen cannot be combined with --from remote (local listeners don't help for internet-side checks).")
	}

	targets, err := buildManualPortCheckTargets(portValue, protocol)
	if err != nil {
		return err
	}

	return withProbeListeners(d.HostDeps, targets, listen, func() error {
		return runPortChecks(d, side, targets, hostAddr)
	})
}

func runInstanceMode(d *core.AppDeps, instance, from string, listen bool, composeFileFlag, portsFlag string) error {
	side, hostAddr := parseFromFlag(from)

	if listen && side == host.CheckSideInternet {
		return errors.New("Error: --listen cannot be combined with --from remote (local listeners don't help for internet-side checks).")
	}

	targets, err := buildInstancePortCheckTargets(d, composeFileFlag, instance, portsFlag, !listen)
	if err != nil {
		return err
	}

	return withProbeListeners(d.HostDeps, targets, listen, func() error {
		return runPortChecks(d, side, targets, hostAddr)
	})
}

func parseFromFlag(value string) (host.PortCheckSide, string) {
	normalized := strings.ToLower(strings.TrimSpace(value))
	switch normalized {
	case "local":
		return host.CheckSideLocalhost, ""
	case "remote":
		return host.CheckSideInternet, ""
	case "both":
		return host.CheckSideBoth, ""
	default:
		return host.CheckSideHost, normalized
	}
}

func buildManualPortCheckTargets(portValue, protocol string) ([]host.PortCheckTarget, error) {
	parsed, err := strconv.Atoi(portValue)
	if err != nil {
		return nil, fmt.Errorf("Error: invalid port: %s", portValue)
	}
	manualProtocol := strings.ToLower(strings.TrimSpace(protocol))
	switch manualProtocol {
	case "both":
		return []host.PortCheckTarget{
			{Label: "MANUAL", Protocol: host.ProtocolTCP, HostPort: parsed},
			{Label: "MANUAL", Protocol: host.ProtocolUDP, HostPort: parsed},
		}, nil
	case host.ProtocolTCP:
		return []host.PortCheckTarget{{Label: "MANUAL", Protocol: host.ProtocolTCP, HostPort: parsed}}, nil
	case host.ProtocolUDP:
		return []host.PortCheckTarget{{Label: "MANUAL", Protocol: host.ProtocolUDP, HostPort: parsed}}, nil
	default:
		return nil, errors.New("Error: --protocol must be one of: tcp, udp, both.")
	}
}

func buildInstancePortCheckTargets(d *core.AppDeps, composeFileFlag, instance, which string, requireRunning bool) ([]host.PortCheckTarget, error) {
	composeFile, config, err := compose.ResolveRequiredServiceConfig(d, composeFileFlag, instance, compose.ResolveRequiredComposeConfig)
	if err != nil {
		return nil, err
	}
	if requireRunning {
		if err := d.RuntimeHelper.EnsureServiceRunning(composeFile, instance); err != nil {
			return nil, err
		}
	}

	service := config.Project.Services[instance]
	asaPort := runtime.ServiceASAPort(service)
	if asaPort <= 0 {
		return nil, fmt.Errorf("Error: failed to resolve ASA game port from compose config")
	}
	rconPort, err := host.ResolveHostPortByTarget(service.Ports, 27020, "tcp")
	if err != nil {
		return nil, fmt.Errorf("Error: failed to resolve RCON port from compose config: %v", err)
	}
	targets := []host.PortCheckTarget{
		{Label: "ASA", Protocol: host.ProtocolUDP, ContainerPort: asaPort, HostPort: asaPort},
		{Label: "RCON", Protocol: host.ProtocolTCP, ContainerPort: 27020, HostPort: rconPort},
	}
	switch strings.ToLower(strings.TrimSpace(which)) {
	case "all":
	case "asa":
		targets = targets[:1]
	case "rcon":
		targets = targets[1:]
	default:
		return nil, errors.New("Error: --ports must be one of: asa, rcon, all.")
	}
	if targets[len(targets)-1].Label == "RCON" && rconLocalOnly(service.Ports) {
		core.Infof("Note: RCON for %s is published on 127.0.0.1 only, so CLOSED from remote or another host is expected.\n"+
			"Run 'ark-cli enable remote-rcon --instance %s' to allow remote RCON access.\n", instance, instance)
	}
	return targets, nil
}

// rconLocalOnly reports whether the RCON binding (container port 27020/tcp) is published on loopback only.
func rconLocalOnly(ports []types.ServicePortConfig) bool {
	for _, p := range ports {
		if p.Target == 27020 && host.NormalizePortProtocol(p.Protocol) == host.ProtocolTCP {
			return net.ParseIP(p.HostIP).IsLoopback()
		}
	}
	return false
}

func withProbeListeners(deps host.HostDeps, targets []host.PortCheckTarget, enabled bool, run func() error) error {
	if !enabled {
		return run()
	}

	cleanups, err := host.OpenProbeListenersForTargets(deps, targets)
	if err != nil {
		return host.AddCheckPortListenerConflictHint(err)
	}
	defer host.CloseProbeListeners(cleanups)
	core.Infoln("Opened temporary local listener(s) for port probe.")
	return run()
}

type portCheckRunner func(target host.PortCheckTarget) (bool, string, string, error)

func runPortChecks(d *core.AppDeps, side host.PortCheckSide, targets []host.PortCheckTarget, hostAddr string) error {
	var internetChecker portCheckRunner
	var publicIP string
	if side&host.CheckSideInternet != 0 {
		ip, err := host.ResolvePublicIP(d.HostDeps)
		if err != nil {
			return err
		}
		publicIP = ip
		client := host.NewPortCheckProviderClient(d.HostDeps, ip)
		internetChecker = client.CheckExternalPortStatus
	}

	rows := make([]host.PortRowResult, len(targets))
	for i, target := range targets {
		rows[i].Target = target
		if side&host.CheckSideInternet != 0 {
			isOpen, provider, raw, err := internetChecker(target)
			if err != nil {
				rows[i].Internet = host.PortSideResult{Status: host.PortStatusError, Provider: provider}
			} else {
				rows[i].Internet = host.PortSideResult{Status: host.ToInternetPortStatus(isOpen, raw), Provider: provider}
			}
		}
		if side&host.CheckSideLocalhost != 0 || side&host.CheckSideHost != 0 {
			isOpen, provider, raw, _ := host.CheckPortLocalhost(target, hostAddr)
			rows[i].Localhost = host.PortSideResult{Status: host.ToLocalhostPortStatus(isOpen, raw), Provider: provider}
		}
	}

	if side&host.CheckSideLocalhost != 0 {
		host.PrintPortPreCheckWarning(os.Stdout, rows)
	}
	host.PrintPortCheckTable(os.Stdout, publicIP, hostAddr, side, rows)
	return nil
}
