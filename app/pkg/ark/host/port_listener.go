package host

import (
	"fmt"
	"net"
	"strings"
)

// OpenTCPProbeListener opens a TCP listener on the given port and returns a
// cleanup function that closes it.
func OpenTCPProbeListener(port int) (func(), error) {
	ln, err := net.Listen(ProtocolTCP, fmt.Sprintf(":%d", port))
	if err != nil {
		return nil, err
	}
	return func() { _ = ln.Close() }, nil
}

// OpenUDPProbeListenerImpl opens a UDP packet listener on the given port that
// echoes any received payload back. It returns a cleanup function that closes
// the connection.
func OpenUDPProbeListenerImpl(port int) (func(), error) {
	pc, err := net.ListenPacket(ProtocolUDP, fmt.Sprintf(":%d", port))
	if err != nil {
		return nil, err
	}
	// Reply to UDP probes so external checkers can classify the port as open.
	go func() {
		buf := make([]byte, 2048)
		for {
			n, addr, err := pc.ReadFrom(buf)
			if err != nil {
				return
			}
			reply := []byte("ark-cli-udp-probe")
			if n > 0 {
				reply = append([]byte(nil), buf[:n]...)
			}
			_, _ = pc.WriteTo(reply, addr)
		}
	}()
	return func() { _ = pc.Close() }, nil
}

// OpenUDPProbeListener opens a UDP probe listener on the given port, using
// deps.OpenUDPProbeListener if set (for testing), otherwise falling back to
// the real implementation.
func OpenUDPProbeListener(deps HostDeps, port int) (func(), error) {
	if dep := deps.OpenUDPProbeListener; dep != nil {
		return dep(port)
	}
	return OpenUDPProbeListenerImpl(port)
}

// AddCheckPortListenerConflictHint augments bind errors that mention "address
// already in use" with a hint about --listener.
func AddCheckPortListenerConflictHint(err error) error {
	if strings.Contains(strings.ToLower(err.Error()), "address already in use") {
		return fmt.Errorf("Error: %v (port already in use; stop the service or omit --listener)", err)
	}
	return err
}

// OpenProbeListenerWithContext opens a probe listener for the given target and
// protocol, wrapping any error with context.
func OpenProbeListenerWithContext(
	target PortCheckTarget,
	protocolLabel string,
	opener func(int) (func(), error),
) (func(), error) {
	closeFn, err := opener(target.HostPort)
	if err != nil {
		return nil, fmt.Errorf(
			"Error: failed to open %s listener on %d for %s probe: %v",
			protocolLabel,
			target.HostPort,
			target.Label,
			err,
		)
	}
	return closeFn, nil
}

// CloseProbeListeners calls each cleanup function in reverse order.
func CloseProbeListeners(cleanups []func()) {
	for i := len(cleanups) - 1; i >= 0; i-- {
		cleanups[i]()
	}
}

// OpenProbeListenersForTarget opens probe listeners (TCP, UDP, or both)
// for a single target, using deps for the UDP listener seam.
func OpenProbeListenersForTarget(deps HostDeps, target PortCheckTarget) ([]func(), error) {
	type listenerOp struct {
		label  string
		opener func(int) (func(), error)
	}

	var ops []listenerOp
	switch target.Protocol {
	case ProtocolUDP:
		ops = []listenerOp{{label: "UDP", opener: func(p int) (func(), error) { return OpenUDPProbeListener(deps, p) }}}
	case ProtocolTCP:
		ops = []listenerOp{{label: "TCP", opener: OpenTCPProbeListener}}
	case "tcp/udp":
		ops = []listenerOp{
			{label: "TCP", opener: OpenTCPProbeListener},
			{label: "UDP", opener: func(p int) (func(), error) { return OpenUDPProbeListener(deps, p) }},
		}
	default:
		return nil, fmt.Errorf("Error: unsupported probe listener protocol: %s", target.Protocol)
	}

	cleanups := make([]func(), 0, len(ops))
	for _, op := range ops {
		closeFn, err := OpenProbeListenerWithContext(target, op.label, op.opener)
		if err != nil {
			CloseProbeListeners(cleanups)
			return nil, err
		}
		cleanups = append(cleanups, closeFn)
	}
	return cleanups, nil
}

// OpenProbeListenersForTargets opens probe listeners for multiple targets,
// rolling back all on failure.
func OpenProbeListenersForTargets(deps HostDeps, targets []PortCheckTarget) ([]func(), error) {
	cleanups := []func(){}
	for _, target := range targets {
		targetCleanups, err := OpenProbeListenersForTarget(deps, target)
		if err != nil {
			CloseProbeListeners(cleanups)
			return nil, err
		}
		cleanups = append(cleanups, targetCleanups...)
	}
	return cleanups, nil
}
