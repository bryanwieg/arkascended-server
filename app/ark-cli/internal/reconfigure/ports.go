package reconfigure

import (
	"fmt"
	"sort"
	"strconv"

	"github.com/compose-spec/compose-go/v2/types"

	"pkg/ark/composeutil"
	"pkg/ark/host"
	"pkg/ark/runtime"
)

const (
	arkContainerPortUDP  = 27015
	rconContainerPortTCP = 27020
	errServicePortsList  = "Error: service ports must be a list."

	labelPortRCON = "ark.port.rcon"

	// rconLocalHostIP keeps RCON reachable only from the Docker host (the default).
	rconLocalHostIP = "127.0.0.1"
)

type reconfigurePortRole string

const (
	reconfigurePortRoleUnknown reconfigurePortRole = "unknown"
	reconfigurePortRoleARK     reconfigurePortRole = "ark"
	reconfigurePortRoleRCON    reconfigurePortRole = "rcon"
)

type reconfigurePortConflict struct {
	service string
	role    reconfigurePortRole
}

type publishedRolePort struct {
	role      reconfigurePortRole
	published int
}

// checkPortConflict returns an error if requestedPort is used by any service other than excluded.
func checkPortConflict(services types.Services, excluded string, port int, label string) error {
	conflict, ok, err := findReconfigurePortConflict(services, excluded, port)
	if err != nil {
		return err
	}
	if ok {
		return reconfigurePortConflictError(label, port, conflict)
	}
	return nil
}

func applyReconfigurePortUpdates(project *types.Project, req reconfigureCommandRequest) (bool, error) {
	if !req.asaPortSet && !req.rconPortSet && !req.rconHostIPSet {
		return false, nil
	}
	_, ok := project.Services[req.instance]
	if !ok {
		return false, fmt.Errorf("Error: service not found in compose file: %s", req.instance)
	}

	if req.asaPortSet {
		if err := checkPortConflict(project.Services, req.instance, req.asaPort, "ARK"); err != nil {
			return false, err
		}
	}
	if req.rconPortSet {
		if err := checkPortConflict(project.Services, req.instance, req.rconPort, "RCON"); err != nil {
			return false, err
		}
	}

	changed := false
	if req.asaPortSet {
		updated, err := upsertServiceASAPort(project, req.instance, req.asaPort)
		if err != nil {
			return false, err
		}
		changed = changed || updated
	}
	if req.rconPortSet {
		updated, err := upsertServiceRolePublishedPort(project, req.instance, reconfigurePortRoleRCON, req.rconPort)
		if err != nil {
			return false, err
		}
		changed = changed || updated
	}
	if req.rconHostIPSet {
		updated, err := setRCONHostIP(project, req.instance, req.rconHostIP)
		if err != nil {
			return false, err
		}
		changed = changed || updated
	}
	return changed, nil
}

// setRCONHostIP sets the host IP of the instance's RCON binding (container port 27020/tcp).
// An empty hostIP publishes on all interfaces, IPv4 and IPv6; "0.0.0.0" would be IPv4 only.
func setRCONHostIP(project *types.Project, serviceName, hostIP string) (bool, error) {
	found, changed := false, false
	err := composeutil.MutateService(project, serviceName, func(s *types.ServiceConfig) {
		for i, p := range s.Ports {
			if roleForTargetProtocol(int(p.Target), host.NormalizePortProtocol(p.Protocol)) != reconfigurePortRoleRCON {
				continue
			}
			found = true
			if p.HostIP != hostIP {
				s.Ports[i].HostIP = hostIP
				changed = true
			}
		}
	})
	if err == nil && !found {
		err = fmt.Errorf("Error: no RCON port binding (container port %d/tcp) found for instance %s.", rconContainerPortTCP, serviceName)
	}
	return changed, err
}

// upsertServiceASAPort updates the ASA game port in a service definition.
//
// New-scheme services (with ark.port.asa label): the port binding is 1:1
// (published == target). Both the binding, the label, and the ASA_PORT env var
// are kept in sync.
//
// Legacy services (no label): only the published side of the binding is
// updated; the fixed container target (27015) is preserved unchanged.
func upsertServiceASAPort(project *types.Project, serviceName string, newPort int) (bool, error) {
	svc := project.Services[serviceName]
	newScheme := svc.Labels[runtime.LabelPortASA] != ""

	// Determine the target port to search for in existing bindings.
	currentPort := uint32(arkContainerPortUDP)
	if newScheme {
		if raw := svc.Labels[runtime.LabelPortASA]; raw != "" {
			if port, err := strconv.Atoi(raw); err == nil && port > 0 {
				currentPort = uint32(port)
			}
		}
	}

	var updated bool
	err := composeutil.MutateService(project, serviceName, func(s *types.ServiceConfig) {
		found := false
		for i, p := range s.Ports {
			if host.NormalizePortProtocol(p.Protocol) != "udp" || p.Target != currentPort {
				continue
			}
			if newScheme {
				s.Ports[i].Target = uint32(newPort)
			}
			s.Ports[i].Published = strconv.Itoa(newPort)
			pub, _ := composeutil.PublishedPort(p)
			updated = pub != newPort || (newScheme && p.Target != uint32(newPort))
			found = true
			break
		}
		if !found {
			target := uint32(arkContainerPortUDP)
			if newScheme {
				target = uint32(newPort)
			}
			s.Ports = append(s.Ports, types.ServicePortConfig{
				Target:    target,
				Published: strconv.Itoa(newPort),
				Protocol:  "udp",
			})
			updated = true
		}
		if newScheme {
			if s.Labels == nil {
				s.Labels = types.Labels{}
			}
			s.Labels[runtime.LabelPortASA] = strconv.Itoa(newPort)
			if s.Environment == nil {
				s.Environment = types.MappingWithEquals{}
			}
			composeutil.SetEnv(s.Environment, "ASA_PORT", strconv.Itoa(newPort))
		}
	})
	return updated, err
}

func findReconfigurePortConflict(
	services types.Services,
	excludedService string,
	requestedPort int,
) (reconfigurePortConflict, bool, error) {
	serviceNames := make([]string, 0, len(services))
	for name := range services {
		serviceNames = append(serviceNames, name)
	}
	sort.Strings(serviceNames)

	for _, serviceName := range serviceNames {
		if serviceName == excludedService {
			continue
		}
		svc := services[serviceName]
		rolePorts := publishedRolePortsForService(svc)
		for _, rolePort := range rolePorts {
			if rolePort.published == requestedPort {
				return reconfigurePortConflict{service: serviceName, role: rolePort.role}, true, nil
			}
		}
	}
	return reconfigurePortConflict{}, false, nil
}

// publishedRolePortsForService returns published port + role pairs for a service.
// Prefers label-based identification (new scheme); falls back to binding target
// for legacy services without labels.
func publishedRolePortsForService(svc types.ServiceConfig) []publishedRolePort {
	asaLabel := svc.Labels[runtime.LabelPortASA]
	rconLabel := svc.Labels[labelPortRCON]
	if asaLabel != "" || rconLabel != "" {
		var result []publishedRolePort
		if asaLabel != "" {
			if port, err := strconv.Atoi(asaLabel); err == nil && port > 0 {
				result = append(result, publishedRolePort{role: reconfigurePortRoleARK, published: port})
			}
		}
		if rconLabel != "" {
			if port, err := strconv.Atoi(rconLabel); err == nil && port > 0 {
				result = append(result, publishedRolePort{role: reconfigurePortRoleRCON, published: port})
			}
		}
		return result
	}
	return servicePublishedRolePortsFromBindings(svc)
}

// servicePublishedRolePortsFromBindings scans port bindings to infer roles by
// target port (legacy behaviour for compose files without ark labels).
func servicePublishedRolePortsFromBindings(svc types.ServiceConfig) []publishedRolePort {
	var rolePorts []publishedRolePort
	for _, p := range svc.Ports {
		role := roleForTargetProtocol(int(p.Target), host.NormalizePortProtocol(p.Protocol))
		if role == reconfigurePortRoleUnknown {
			continue
		}
		published, ok := composeutil.PublishedPort(p)
		if !ok {
			continue
		}
		rolePorts = append(rolePorts, publishedRolePort{role: role, published: published})
	}
	return rolePorts
}

func upsertServiceRolePublishedPort(project *types.Project, serviceName string, role reconfigurePortRole, publishedPort int) (bool, error) {
	svc := project.Services[serviceName]
	var updated bool
	err := composeutil.MutateService(project, serviceName, func(s *types.ServiceConfig) {
		for i, p := range s.Ports {
			pRole := roleForTargetProtocol(int(p.Target), host.NormalizePortProtocol(p.Protocol))
			if pRole != role {
				continue
			}
			current, _ := composeutil.PublishedPort(p)
			s.Ports[i].Published = strconv.Itoa(publishedPort)
			updated = current != publishedPort
			if role == reconfigurePortRoleRCON && svc.Labels[labelPortRCON] != "" {
				if s.Labels == nil {
					s.Labels = types.Labels{}
				}
				s.Labels[labelPortRCON] = strconv.Itoa(publishedPort)
			}
			return
		}
		// No existing binding matched: append a new one.
		s.Ports = append(s.Ports, defaultPublishedRolePortConfig(role, publishedPort))
		if role == reconfigurePortRoleRCON && svc.Labels[labelPortRCON] != "" {
			if s.Labels == nil {
				s.Labels = types.Labels{}
			}
			s.Labels[labelPortRCON] = strconv.Itoa(publishedPort)
		}
		updated = true
	})
	return updated, err
}

func defaultPublishedRolePortConfig(role reconfigurePortRole, publishedPort int) types.ServicePortConfig {
	switch role {
	case reconfigurePortRoleARK:
		return types.ServicePortConfig{
			Target:    uint32(publishedPort),
			Published: strconv.Itoa(publishedPort),
			Protocol:  "udp",
		}
	case reconfigurePortRoleRCON:
		return types.ServicePortConfig{
			HostIP:    rconLocalHostIP, // new bindings follow the local-only default
			Target:    rconContainerPortTCP,
			Published: strconv.Itoa(publishedPort),
			Protocol:  "tcp",
		}
	default:
		return types.ServicePortConfig{
			Published: strconv.Itoa(publishedPort),
		}
	}
}

func reconfigurePortConflictError(requestedLabel string, requestedPort int, conflict reconfigurePortConflict) error {
	return fmt.Errorf(
		"Error: requested %s port %d is already used by instance %s for %s.",
		requestedLabel,
		requestedPort,
		conflict.service,
		conflict.role,
	)
}

func roleForTargetProtocol(target int, protocol string) reconfigurePortRole {
	switch {
	case target == arkContainerPortUDP && protocol == "udp":
		return reconfigurePortRoleARK
	case target == rconContainerPortTCP && protocol == "tcp":
		return reconfigurePortRoleRCON
	default:
		return reconfigurePortRoleUnknown
	}
}
