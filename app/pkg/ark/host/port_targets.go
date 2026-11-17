package host

import (
	"fmt"
	"strconv"
	"strings"

	"pkg/ark/composeutil"

	"github.com/compose-spec/compose-go/v2/types"
)

const (
	// ProtocolTCP is the TCP protocol identifier for port check targets.
	ProtocolTCP = "tcp"
	// ProtocolUDP is the UDP protocol identifier for port check targets.
	ProtocolUDP = "udp"
)

// PortCheckTarget describes a single port to be probed.
type PortCheckTarget struct {
	Label         string
	Protocol      string
	ContainerPort int
	HostPort      int
}

// ResolveUDPHostPort resolves the published host port for the UDP game port
// (container target 27015) from a compose service's port mappings.
func ResolveUDPHostPort(ports []types.ServicePortConfig) (int, error) {
	return ResolveHostPortByTarget(ports, 27015, ProtocolUDP)
}

// ResolveHostPortByTarget finds the published host port for a given container
// target port and protocol from compose service port config.
func ResolveHostPortByTarget(ports []types.ServicePortConfig, targetPort int, protocol string) (int, error) {
	normalizedProtocol := NormalizePortProtocol(protocol)
	for _, p := range ports {
		pProto := NormalizePortProtocol(p.Protocol)
		if pProto != normalizedProtocol {
			continue
		}
		if int(p.Target) != targetPort {
			continue
		}
		published, ok := composeutil.PublishedPort(p)
		if !ok {
			continue
		}
		return published, nil
	}
	return 0, fmt.Errorf(
		"Error: failed to resolve exposed %s port for container port %d from compose config",
		protocol,
		targetPort,
	)
}

// NormalizePortProtocol returns a normalized protocol string ("tcp" or "udp").
func NormalizePortProtocol(protocol string) string {
	normalized := strings.ToLower(strings.TrimSpace(protocol))
	if normalized == "" {
		return ProtocolTCP
	}
	return normalized
}

// ParseShortComposePortMapping parses a "hostPort:containerPort[/proto]" string
// and returns (containerPort, hostPort, true) on success.
func ParseShortComposePortMapping(value string) (int, int, bool) {
	raw := strings.SplitN(value, "/", 2)[0]
	parts := strings.Split(raw, ":")
	if len(parts) < 2 {
		return 0, 0, false
	}
	containerPort, err := strconv.Atoi(parts[len(parts)-1])
	if err != nil {
		return 0, 0, false
	}
	hostPort, err := strconv.Atoi(parts[len(parts)-2])
	if err != nil {
		return 0, 0, false
	}
	return containerPort, hostPort, true
}
