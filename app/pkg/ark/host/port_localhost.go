package host

import (
	"fmt"
	"net"
	"time"
)

const localhostProbeHost = "127.0.0.1"

// CheckPortLocalhost checks whether a port is reachable on the local host.
// If host is empty, "127.0.0.1" is used.
// Returns (isOpen, host, statusDescription, error).
func CheckPortLocalhost(target PortCheckTarget, host string) (bool, string, string, error) {
	if host == "" {
		host = localhostProbeHost
	}
	switch target.Protocol {
	case ProtocolTCP:
		if err := probeLocalTCPPort(host, target.HostPort); err != nil {
			return false, host, fmt.Sprintf("closed (%v)", err), nil
		}
		return true, host, "open", nil
	case ProtocolUDP:
		status := checkLocalhostUDPStatus(host, target.HostPort)
		return status == "open", host, status, nil
	default:
		return false, host, "", fmt.Errorf("Error: unsupported protocol for localhost check: %s", target.Protocol)
	}
}

func probeLocalTCPPort(host string, port int) error {
	conn, err := net.DialTimeout(ProtocolTCP, fmt.Sprintf("%s:%d", host, port), 500*time.Millisecond)
	if err != nil {
		return err
	}
	_ = conn.Close()
	return nil
}

// checkLocalhostUDPStatus probes a UDP port and returns one of: "open",
// "listening", or "closed".
//
// "open"      — a response was received.  The UE5 stateless connect handshake
//
//	is tried first; it elicits a reply from a running ARK server
//	and from the CLI's own --listener echo service.
//
// "listening" — the port is bound but no response was received (would be seen
//
//	with a non-ARK UDP process that ignores unknown probes).
//
// "closed"    — nothing is bound on the port.
func checkLocalhostUDPStatus(host string, port int) string {
	if ProbeUE5UDP(host, port, 500*time.Millisecond) {
		return "open"
	}

	// No response — try to bind to the port to determine whether anything is
	// listening.  If the bind succeeds the port is free (CLOSED); if it fails
	// with "address already in use" something is bound (LISTENING).
	addr := fmt.Sprintf("%s:%d", host, port)
	pc, bindErr := net.ListenPacket(ProtocolUDP, addr)
	if bindErr == nil {
		_ = pc.Close()
		return "closed"
	}
	return "listening"
}
