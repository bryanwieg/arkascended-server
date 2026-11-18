package host

import (
	"fmt"
	"net"
	"strings"
	"testing"
	"time"

	"github.com/compose-spec/compose-go/v2/types"
)

func skipIfSocketDenied(t *testing.T, err error) {
	t.Helper()
	if err != nil && strings.Contains(strings.ToLower(err.Error()), "operation not permitted") {
		t.Skipf("socket operations not permitted in this environment: %v", err)
	}
}

func pickFreeTCPAndUDPPort(t *testing.T) int {
	t.Helper()
	for i := 0; i < 20; i++ {
		tcpLn, err := net.Listen("tcp", "127.0.0.1:0")
		if err != nil {
			skipIfSocketDenied(t, err)
			t.Fatalf("listen tcp: %v", err)
		}
		port := tcpLn.Addr().(*net.TCPAddr).Port
		_ = tcpLn.Close()
		udpPc, err := net.ListenPacket("udp", fmt.Sprintf("127.0.0.1:%d", port))
		if err != nil {
			continue
		}
		_ = udpPc.Close()
		return port
	}
	t.Fatal("failed to find free port for both tcp and udp")
	return 0
}

func pickFreeTCPPort(t *testing.T) int {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		skipIfSocketDenied(t, err)
		t.Fatalf("listen tcp: %v", err)
	}
	defer ln.Close()
	return ln.Addr().(*net.TCPAddr).Port
}

func pickFreeUDPPort(t *testing.T) int {
	t.Helper()
	pc, err := net.ListenPacket("udp", "127.0.0.1:0")
	if err != nil {
		skipIfSocketDenied(t, err)
		t.Fatalf("listen udp: %v", err)
	}
	defer pc.Close()
	return pc.LocalAddr().(*net.UDPAddr).Port
}

func TestResolveHostPortByTarget(t *testing.T) {
	tests := []struct {
		name       string
		ports      []types.ServicePortConfig
		targetPort int
		protocol   string
		wantPort   int
	}{
		{
			name: "udp target",
			ports: []types.ServicePortConfig{
				{Protocol: "udp", Target: 27015, Published: "27025"},
				{Protocol: "tcp", Target: 27020, Published: "27030"},
			},
			targetPort: 27015,
			protocol:   "udp",
			wantPort:   27025,
		},
		{
			name: "tcp target",
			ports: []types.ServicePortConfig{
				{Protocol: "udp", Target: 27015, Published: "27025"},
				{Protocol: "tcp", Target: 27020, Published: "27030"},
			},
			targetPort: 27020,
			protocol:   "tcp",
			wantPort:   27030,
		},
		{
			name: "udp target with host ip",
			ports: []types.ServicePortConfig{
				{Protocol: "udp", Target: 27015, Published: "27016"},
				{Protocol: "tcp", Target: 27020, Published: "27021", HostIP: "127.0.0.1"},
			},
			targetPort: 27015,
			protocol:   "udp",
			wantPort:   27016,
		},
		{
			name: "tcp target with host ip",
			ports: []types.ServicePortConfig{
				{Protocol: "udp", Target: 27015, Published: "27016"},
				{Protocol: "tcp", Target: 27020, Published: "27021", HostIP: "127.0.0.1"},
			},
			targetPort: 27020,
			protocol:   "tcp",
			wantPort:   27021,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ResolveHostPortByTarget(tt.ports, tt.targetPort, tt.protocol)
			if err != nil {
				t.Fatalf("ResolveHostPortByTarget: %v", err)
			}
			if got != tt.wantPort {
				t.Fatalf("expected %d, got %d", tt.wantPort, got)
			}
		})
	}
}

func TestResolveUDPHostPort(t *testing.T) {
	tests := []struct {
		name    string
		ports   []types.ServicePortConfig
		want    int
		wantErr bool
	}{
		{
			name:  "udp entry",
			ports: []types.ServicePortConfig{{Protocol: "udp", Target: 27015, Published: "27015"}},
			want:  27015,
		},
		{
			name: "multiple udp entries",
			ports: []types.ServicePortConfig{
				{Protocol: "udp", Target: 27015, Published: "27016"},
				{Protocol: "udp", Target: 27015, Published: "27017", HostIP: "127.0.0.1"},
			},
			want: 27016,
		},
		{
			name:    "missing udp",
			ports:   []types.ServicePortConfig{{Protocol: "tcp", Target: 27015, Published: "27015"}},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ResolveUDPHostPort(tt.ports)
			if tt.wantErr {
				if err == nil {
					t.Fatal("expected error")
				}
				return
			}
			if err != nil {
				t.Fatalf("ResolveUDPHostPort: %v", err)
			}
			if got != tt.want {
				t.Fatalf("expected %d, got %d", tt.want, got)
			}
		})
	}
}

func TestCheckLocalhostUDPStatusOpen(t *testing.T) {
	// Start a UDP echo listener so the probe gets a response → OPEN.
	pc, err := net.ListenPacket("udp", "127.0.0.1:0")
	if err != nil {
		skipIfSocketDenied(t, err)
		t.Fatalf("listen udp: %v", err)
	}
	port := pc.LocalAddr().(*net.UDPAddr).Port
	go func() {
		buf := make([]byte, 256)
		n, addr, err := pc.ReadFrom(buf)
		if err != nil {
			return
		}
		_, _ = pc.WriteTo(buf[:n], addr)
	}()
	defer pc.Close()

	status := checkLocalhostUDPStatus("127.0.0.1", port)
	if status != "open" {
		t.Fatalf("expected open with echo listener, got %q", status)
	}
}

func TestCheckLocalhostUDPStatusListening(t *testing.T) {
	// Bind a UDP socket that does NOT reply → LISTENING.
	pc, err := net.ListenPacket("udp", "127.0.0.1:0")
	if err != nil {
		skipIfSocketDenied(t, err)
		t.Fatalf("listen udp: %v", err)
	}
	port := pc.LocalAddr().(*net.UDPAddr).Port
	defer pc.Close()

	status := checkLocalhostUDPStatus("127.0.0.1", port)
	if status != "listening" {
		t.Fatalf("expected listening for silent bound port, got %q", status)
	}
}

func TestCheckLocalhostUDPStatusClosed(t *testing.T) {
	port := pickFreeUDPPort(t)
	status := checkLocalhostUDPStatus("127.0.0.1", port)
	if status != "closed" {
		t.Fatalf("expected closed for unbound port, got %q", status)
	}
}

func TestOpenTCPProbeListener(t *testing.T) {
	port := pickFreeTCPPort(t)
	closeFn, err := OpenTCPProbeListener(port)
	if err != nil {
		t.Fatalf("OpenTCPProbeListener: %v", err)
	}
	defer closeFn()

	// Verify the port is actually bound.
	conn, err := net.DialTimeout("tcp", fmt.Sprintf("127.0.0.1:%d", port), 300*time.Millisecond)
	if err != nil {
		t.Fatalf("expected port %d to be open, got: %v", port, err)
	}
	_ = conn.Close()
}

func TestOpenUDPProbeListener(t *testing.T) {
	deps := HostDeps{} // no seam override, uses real impl
	port := pickFreeUDPPort(t)
	closeFn, err := OpenUDPProbeListener(deps, port)
	if err != nil {
		t.Fatalf("OpenUDPProbeListener: %v", err)
	}
	defer closeFn()

	// Verify the port is bound by trying to send a packet and get a reply.
	conn, err := net.DialTimeout("udp", fmt.Sprintf("127.0.0.1:%d", port), 1*time.Second)
	if err != nil {
		t.Fatalf("dial udp: %v", err)
	}
	defer conn.Close()
	_ = conn.SetDeadline(time.Now().Add(2 * time.Second))
	if _, err := conn.Write([]byte("test")); err != nil {
		t.Fatalf("write udp: %v", err)
	}
	buf := make([]byte, 256)
	n, err := conn.Read(buf)
	if err != nil {
		t.Fatalf("read udp: %v", err)
	}
	if n == 0 {
		t.Fatal("expected response from UDP probe listener")
	}
}

func TestOpenProbeListenersForTargetTCP(t *testing.T) {
	deps := HostDeps{}
	port := pickFreeTCPPort(t)
	target := PortCheckTarget{Label: "test", Protocol: ProtocolTCP, HostPort: port}
	cleanups, err := OpenProbeListenersForTarget(deps, target)
	if err != nil {
		t.Fatalf("OpenProbeListenersForTarget: %v", err)
	}
	defer CloseProbeListeners(cleanups)

	// Verify TCP listener is running.
	conn, err := net.DialTimeout("tcp", fmt.Sprintf("127.0.0.1:%d", port), 300*time.Millisecond)
	if err != nil {
		t.Fatalf("expected port %d to be open, got: %v", port, err)
	}
	_ = conn.Close()
}

func TestAddCheckPortListenerConflictHint(t *testing.T) {
	err := fmt.Errorf("bind: address already in use")
	hinted := AddCheckPortListenerConflictHint(err)
	if !strings.Contains(hinted.Error(), "omit --listen") {
		t.Fatalf("expected hint about --listen, got: %v", hinted)
	}

	plain := fmt.Errorf("some other error")
	hinted2 := AddCheckPortListenerConflictHint(plain)
	if hinted2.Error() != plain.Error() {
		t.Fatalf("expected unchanged error, got: %v", hinted2)
	}
}

func TestColorizePortStatus(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{PortStatusOpen, ansiGreen + PortStatusOpen + ansiReset},
		{PortStatusClosed, ansiRed + PortStatusClosed + ansiReset},
		{PortStatusError, ansiRed + PortStatusError + ansiReset},
		{PortStatusUnknown, ansiYellow + PortStatusUnknown + ansiReset},
		{PortStatusListening, ansiYellow + PortStatusListening + ansiReset},
		{"other", "other"},
	}
	for _, tt := range tests {
		got := ColorizePortStatus(tt.input)
		if got != tt.want {
			t.Errorf("ColorizePortStatus(%q) = %q, want %q", tt.input, got, tt.want)
		}
	}
}

func TestToInternetPortStatus(t *testing.T) {
	tests := []struct {
		isOpen bool
		raw    string
		want   string
	}{
		{true, "", PortStatusOpen},
		{false, "open", PortStatusOpen},
		{false, "partially open", PortStatusOpen},
		{false, "closed", PortStatusClosed},
		{false, "connection rejected", PortStatusClosed},
		{false, "something else", PortStatusUnknown},
	}
	for _, tt := range tests {
		got := ToInternetPortStatus(tt.isOpen, tt.raw)
		if got != tt.want {
			t.Errorf("ToInternetPortStatus(%v, %q) = %q, want %q", tt.isOpen, tt.raw, got, tt.want)
		}
	}
}

func TestToLocalhostPortStatus(t *testing.T) {
	tests := []struct {
		isOpen bool
		raw    string
		want   string
	}{
		{true, "", PortStatusOpen},
		{false, "open", PortStatusOpen},
		{false, "listening", PortStatusListening},
		{false, "closed", PortStatusClosed},
		{false, "something", PortStatusUnknown},
	}
	for _, tt := range tests {
		got := ToLocalhostPortStatus(tt.isOpen, tt.raw)
		if got != tt.want {
			t.Errorf("ToLocalhostPortStatus(%v, %q) = %q, want %q", tt.isOpen, tt.raw, got, tt.want)
		}
	}
}

func TestPrintPortCheckTableOutput(t *testing.T) {
	rows := []PortRowResult{
		{
			Target:    PortCheckTarget{Label: "ASA", Protocol: ProtocolUDP, HostPort: 27015},
			Internet:  PortSideResult{Status: PortStatusOpen, Provider: "starrupture-utilities.com"},
			Localhost: PortSideResult{Status: PortStatusOpen},
		},
		{
			Target:    PortCheckTarget{Label: "RCON", Protocol: ProtocolTCP, HostPort: 27020},
			Internet:  PortSideResult{Status: PortStatusClosed, Provider: "starrupture-utilities.com"},
			Localhost: PortSideResult{Status: PortStatusClosed},
		},
	}
	var buf strings.Builder
	PrintPortCheckTable(&buf, "1.2.3.4", "", CheckSideBoth, rows)
	output := buf.String()
	if !strings.Contains(output, "ASA") || !strings.Contains(output, "27015") {
		t.Fatalf("missing ASA row: %s", output)
	}
	if !strings.Contains(output, "INTERNET (1.2.3.4)") {
		t.Fatalf("expected INTERNET column with IP: %s", output)
	}
	if !strings.Contains(output, "LOCALHOST") {
		t.Fatalf("expected LOCALHOST column: %s", output)
	}
}

func TestPrintPortPreCheckWarningClosed(t *testing.T) {
	rows := []PortRowResult{
		{
			Target:    PortCheckTarget{Label: "ASA", Protocol: ProtocolUDP, HostPort: 27015},
			Localhost: PortSideResult{Status: PortStatusClosed},
		},
	}
	var buf strings.Builder
	PrintPortPreCheckWarning(&buf, rows)
	output := buf.String()
	if !strings.Contains(output, "--listen ") {
		t.Fatalf("expected listener hint for closed port, got: %s", output)
	}
}

func TestPrintPortPreCheckWarningOpen(t *testing.T) {
	rows := []PortRowResult{
		{
			Target:    PortCheckTarget{Label: "ASA", Protocol: ProtocolUDP, HostPort: 27015},
			Localhost: PortSideResult{Status: PortStatusOpen},
		},
	}
	var buf strings.Builder
	PrintPortPreCheckWarning(&buf, rows)
	output := buf.String()
	if output != "" {
		t.Fatalf("expected no warning for open port, got: %s", output)
	}
}

func TestCheckLocalhostUDPStatusRemoteSilentIsUnknown(t *testing.T) {
	// 192.0.2.1 (TEST-NET-1) is never a local address: no reply and no bind test.
	status := checkLocalhostUDPStatus("192.0.2.1", 7777)
	if status != "unknown" {
		t.Fatalf("expected unknown for silent remote port, got %q", status)
	}
}
