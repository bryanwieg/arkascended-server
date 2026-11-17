package listenport

import (
	"errors"
	"fmt"
	"os"
	"os/signal"
	"strings"
	"syscall"

	"ark-cli/internal/core"

	"pkg/ark/host"
)

func RunListenPort(d *core.AppDeps, args []string) error {
	fs := core.NewCommandFlagSet(
		"listen_port",
		[]string{
			"listen port --port <num>",
		},
		"Open listeners on a port and wait for Ctrl+C (no probe; for firewall testing).",
	)
	protocolFlag := fs.String("protocol", "both", "Protocol to listen on: tcp, udp, or both.")
	portFlag := fs.Int("port", 0, "Port number to listen on.")

	if err := core.ParseNoPositionalArgs(fs, args); err != nil {
		return err
	}
	if *portFlag <= 0 {
		return errors.New("Error: --port is required.")
	}
	port := *portFlag

	protocol := strings.ToLower(strings.TrimSpace(*protocolFlag))
	var targets []host.PortCheckTarget
	switch protocol {
	case "both":
		targets = []host.PortCheckTarget{
			{Label: "LISTEN", Protocol: host.ProtocolTCP, HostPort: port},
			{Label: "LISTEN", Protocol: host.ProtocolUDP, HostPort: port},
		}
	case host.ProtocolTCP:
		targets = []host.PortCheckTarget{{Label: "LISTEN", Protocol: host.ProtocolTCP, HostPort: port}}
	case host.ProtocolUDP:
		targets = []host.PortCheckTarget{{Label: "LISTEN", Protocol: host.ProtocolUDP, HostPort: port}}
	default:
		return errors.New("Error: --protocol must be one of: tcp, udp, both.")
	}

	// Register before the listeners open, so a Ctrl+C (or a test's SIGINT) that arrives as soon
	// as the port answers is caught, and unregister on return so no stale handler swallows a
	// later signal.
	ch := make(chan os.Signal, 1)
	signal.Notify(ch, syscall.SIGINT, syscall.SIGTERM)
	defer signal.Stop(ch)

	cleanups, err := host.OpenProbeListenersForTargets(d.HostDeps, targets)
	if err != nil {
		return host.AddCheckPortListenerConflictHint(err)
	}
	defer host.CloseProbeListeners(cleanups)

	rows := [][]string{{"LABEL", "PORT", "PROTO"}}
	for _, t := range targets {
		rows = append(rows, []string{t.Label, fmt.Sprintf("%d", t.HostPort), t.Protocol})
	}
	core.PrintTable(rows)
	core.Infoln("Listeners active. Press Ctrl+C to stop.")
	<-ch
	return nil
}
