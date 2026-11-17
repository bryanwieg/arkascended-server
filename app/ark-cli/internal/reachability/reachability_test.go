package reachability

import (
	"context"
	"errors"
	"fmt"
	"net"
	"strings"
	"testing"

	"ark-cli/internal/core"
	"ark-cli/internal/testutil"

	"pkg/ark"
	"pkg/ark/arktest"

	"github.com/compose-spec/compose-go/v2/types"
)

func skipIfSocketDenied(t *testing.T, err error) {
	t.Helper()
	if err != nil && strings.Contains(strings.ToLower(err.Error()), "operation not permitted") {
		t.Skipf("socket operations not permitted in this environment: %v", err)
	}
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

func captureStdout(t *testing.T, fn func()) string {
	t.Helper()
	return testutil.CaptureStdout(t, fn)
}

func withPatchedAppDeps(t *testing.T, patch func(*core.AppDeps), fn func()) {
	t.Helper()
	testutil.WithPatchedAppDeps(t, testutil.ComposeOnlyConfig, patch, fn)
}

func fakeProject(services types.Services) *ark.ComposeProject {
	return &ark.ComposeProject{
		Project: &types.Project{
			Services: services,
		},
	}
}

func testPorts(portStrs ...string) []types.ServicePortConfig {
	var ports []types.ServicePortConfig
	for _, ps := range portStrs {
		parts := strings.SplitN(ps, "/", 2)
		hostParts := strings.SplitN(parts[0], ":", 2)
		proto := "tcp"
		if len(parts) > 1 {
			proto = parts[1]
		}
		published := hostParts[0]
		containerPort := 0
		fmt.Sscanf(hostParts[1], "%d", &containerPort)
		ports = append(ports, types.ServicePortConfig{
			Protocol:  proto,
			Target:    uint32(containerPort),
			Published: published,
		})
	}
	return ports
}

func withInstanceDeps(
	t *testing.T,
	ports []types.ServicePortConfig,
	ensureRunning func(string, string) error,
	mutate func(*core.AppDeps),
	run func(),
) {
	t.Helper()
	running := map[string]struct{}{"asa1": {}}
	if ensureRunning != nil {
		if ensureRunning("", "") != nil {
			running = map[string]struct{}{}
		}
	}
	withPatchedAppDeps(t, func(d *core.AppDeps) {
		d.ComposeService = &arktest.MockCompose{
			LoadProjectFn: func(_ context.Context, _ string) (*ark.ComposeProject, error) {
				return fakeProject(types.Services{
					"asa1": {Name: "asa1", Ports: ports},
				}), nil
			},
			RunningServicesFn: func(_ context.Context, _ string) (map[string]struct{}, error) {
				return running, nil
			},
		}
		if mutate != nil {
			mutate(d)
		}
	}, run)
}

func TestRunCheckReachabilityPort(t *testing.T) {
	out := captureStdout(t, func() {
		withPatchedAppDeps(t, func(d *core.AppDeps) {
			d.HostDeps.FetchText = func(url string) (string, error) {
				if strings.HasSuffix(url, "/api/me") {
					return "203.0.113.1", nil
				}
				if strings.Contains(url, "starrupture-utilities.com") {
					return `{"success":true,"result":{"tcp":{"is_open":true,"status":"open"},"udp":{"is_open":true,"status":"open"}}}`, nil
				}
				return "open", nil
			}
		}, func() {
			if err := RunCheckReachability(core.CurrentAppDeps(), []string{"--port", "27015"}); err != nil {
				t.Fatalf("RunCheckReachability: %v", err)
			}
		})
	})
	if !strings.Contains(out, "MANUAL") || !strings.Contains(out, "27015") {
		t.Fatalf("unexpected output: %s", out)
	}
	if !strings.Contains(out, "INTERNET") || !strings.Contains(out, "LOCALHOST") {
		t.Fatalf("expected both columns in default mode: %s", out)
	}
}

func TestRunCheckReachabilityPortFromLocal(t *testing.T) {
	out := captureStdout(t, func() {
		if err := RunCheckReachability(core.CurrentAppDeps(), []string{"--port", "27015", "--from", "local"}); err != nil {
			t.Fatalf("RunCheckReachability: %v", err)
		}
	})
	if strings.Contains(out, "INTERNET") {
		t.Fatalf("expected no INTERNET column with --from local, got: %s", out)
	}
	if !strings.Contains(out, "LOCALHOST") {
		t.Fatalf("expected LOCALHOST column, got: %s", out)
	}
}

func TestRunCheckReachabilityPortFromRemote(t *testing.T) {
	out := captureStdout(t, func() {
		withPatchedAppDeps(t, func(d *core.AppDeps) {
			d.HostDeps.FetchText = func(url string) (string, error) {
				if strings.HasSuffix(url, "/api/me") {
					return "203.0.113.1", nil
				}
				return `{"success":true,"result":{"tcp":{"is_open":true,"status":"open"},"udp":{"is_open":true,"status":"open"}}}`, nil
			}
		}, func() {
			if err := RunCheckReachability(core.CurrentAppDeps(), []string{"--port", "8000", "--from", "remote"}); err != nil {
				t.Fatalf("RunCheckReachability: %v", err)
			}
		})
	})
	if strings.Contains(out, "LOCALHOST") {
		t.Fatalf("expected no LOCALHOST column with --from remote, got: %s", out)
	}
	if !strings.Contains(out, "INTERNET") {
		t.Fatalf("expected INTERNET column, got: %s", out)
	}
}

func TestRunCheckReachabilityPortFromAddr(t *testing.T) {
	port := pickFreeTCPPort(t)
	ln, err := net.Listen("tcp", fmt.Sprintf("127.0.0.1:%d", port))
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	defer ln.Close()

	out := captureStdout(t, func() {
		if err := RunCheckReachability(core.CurrentAppDeps(), []string{"--port", fmt.Sprintf("%d", port), "--protocol", "tcp", "--from", "127.0.0.1"}); err != nil {
			t.Fatalf("RunCheckReachability: %v", err)
		}
	})
	if strings.Contains(out, "INTERNET") || strings.Contains(out, "LOCALHOST") {
		t.Fatalf("expected no INTERNET/LOCALHOST columns with --from addr, got: %s", out)
	}
	if !strings.Contains(out, "HOST") {
		t.Fatalf("expected HOST column with --from addr, got: %s", out)
	}
	if !strings.Contains(out, "127.0.0.1") {
		t.Fatalf("expected host address in column header, got: %s", out)
	}
	if !strings.Contains(out, "OPEN") {
		t.Fatalf("expected OPEN for listening port, got: %s", out)
	}
}

func TestRunCheckReachabilityPortWithListen(t *testing.T) {
	port := pickFreeTCPAndUDPPort(t)

	out := captureStdout(t, func() {
		if err := RunCheckReachability(core.CurrentAppDeps(), []string{"--port", fmt.Sprintf("%d", port), "--listen", "--from", "local"}); err != nil {
			t.Fatalf("RunCheckReachability: %v", err)
		}
	})
	if !strings.Contains(out, "Opened temporary local listener(s) for port probe.") {
		t.Fatalf("expected listener banner, got: %s", out)
	}
	if !strings.Contains(out, "OPEN") {
		t.Fatalf("expected OPEN with listener active, got: %s", out)
	}
}

func TestRunCheckReachabilityInstance(t *testing.T) {
	fetchCalls := 0
	out := captureStdout(t, func() {
		withInstanceDeps(t, testPorts("27015:27015/udp", "27020:27020/tcp"), nil, func(d *core.AppDeps) {
			d.HostDeps.FetchText = func(url string) (string, error) {
				fetchCalls++
				switch {
				case strings.HasSuffix(url, "/api/me"):
					return "203.0.113.1", nil
				case strings.Contains(url, "starrupture-utilities.com"):
					return `{"success":true,"result":{"tcp":{"is_open":false,"status":"closed"},"udp":{"is_open":true,"status":"open"}}}`, nil
				case strings.HasSuffix(url, "/api/me/27020"):
					return "closed", nil
				default:
					return "open", nil
				}
			}
		}, func() {
			if err := RunCheckReachability(core.CurrentAppDeps(), []string{"--instance", "asa1"}); err != nil {
				t.Fatalf("RunCheckReachability: %v", err)
			}
		})
	})
	if !strings.Contains(out, "ASA") || !strings.Contains(out, "27015") {
		t.Fatalf("missing ASA row: %s", out)
	}
	if !strings.Contains(out, "RCON") || !strings.Contains(out, "27020") {
		t.Fatalf("missing RCON row: %s", out)
	}
	if !strings.Contains(out, "INTERNET") || !strings.Contains(out, "LOCALHOST") {
		t.Fatalf("expected both column headers: %s", out)
	}
	if !strings.Contains(out, "OPEN") {
		t.Fatalf("expected at least one OPEN result: %s", out)
	}
	if !strings.Contains(out, "CLOSED") {
		t.Fatalf("expected at least one CLOSED result: %s", out)
	}
	if fetchCalls < 3 {
		t.Fatalf("expected ip + 2 port checks, got %d fetch calls", fetchCalls)
	}
}

func TestRunCheckReachabilityInstancePortsASA(t *testing.T) {
	fetchCalls := 0
	out := captureStdout(t, func() {
		withInstanceDeps(t, testPorts("27015:27015/udp", "27020:27020/tcp"), nil, func(d *core.AppDeps) {
			d.HostDeps.FetchText = func(url string) (string, error) {
				fetchCalls++
				switch {
				case strings.HasSuffix(url, "/api/me"):
					return "203.0.113.1", nil
				case strings.Contains(url, "starrupture-utilities.com"):
					return `{"success":true,"result":{"tcp":{"is_open":false,"status":"closed"},"udp":{"is_open":true,"status":"open"}}}`, nil
				default:
					return "open", nil
				}
			}
		}, func() {
			if err := RunCheckReachability(core.CurrentAppDeps(), []string{"--instance", "asa1", "--ports", "asa"}); err != nil {
				t.Fatalf("RunCheckReachability: %v", err)
			}
		})
	})
	if !strings.Contains(out, "ASA") || strings.Contains(out, "RCON") {
		t.Fatalf("unexpected output for --ports asa: %s", out)
	}
	if fetchCalls != 2 {
		t.Fatalf("expected 2 fetch calls, got %d", fetchCalls)
	}
}

func TestRunCheckReachabilityInvalidCombinations(t *testing.T) {
	tests := []struct {
		name string
		args []string
	}{
		{name: "missing target", args: []string{}},
		{name: "bad subcommand", args: []string{"--port", "27015", "--protocol", "icmp"}},
		{name: "bad protocol", args: []string{"--port", "8000", "--protocol", "icmp"}},
		{name: "ports flag in port mode", args: []string{"--port", "8000", "--ports", "asa"}},
		{name: "listen with from remote", args: []string{"--port", "8000", "--listen", "--from", "remote"}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			withPatchedAppDeps(t, nil, func() {
				err := RunCheckReachability(core.CurrentAppDeps(), tt.args)
				if err == nil {
					t.Fatal("expected error")
				}
			})
		})
	}
}

func TestRunCheckReachabilityFetchError(t *testing.T) {
	withPatchedAppDeps(t, func(d *core.AppDeps) {
		d.HostDeps.FetchText = func(_ string) (string, error) { return "", errors.New("boom") }
	}, func() {
		err := RunCheckReachability(core.CurrentAppDeps(), []string{"--port", "27015"})
		if err == nil {
			t.Fatal("expected error")
		}
	})
}

func TestRunCheckReachabilityTCPFallback(t *testing.T) {
	out := captureStdout(t, func() {
		withPatchedAppDeps(t, func(d *core.AppDeps) {
			d.HostDeps.FetchText = func(url string) (string, error) {
				switch {
				case strings.HasSuffix(url, "/api/me"):
					return "203.0.113.1", nil
				case strings.Contains(url, "starrupture-utilities.com"):
					return "", errors.New("provider down")
				case strings.HasSuffix(url, "/api/me/8000"):
					return "open", nil
				default:
					return "", errors.New("unexpected url: " + url)
				}
			}
		}, func() {
			if err := RunCheckReachability(core.CurrentAppDeps(), []string{"--port", "8000", "--protocol", "tcp"}); err != nil {
				t.Fatalf("RunCheckReachability: %v", err)
			}
		})
	})
	if !strings.Contains(out, "OPEN") {
		t.Fatalf("expected OPEN via portchecker.io fallback, got: %s", out)
	}
}

func TestRunCheckReachabilityInstanceWithListenSkipsServiceRunning(t *testing.T) {
	udpPort := pickFreeUDPPort(t)
	tcpPort := pickFreeTCPPort(t)

	out := captureStdout(t, func() {
		withInstanceDeps(
			t,
			[]types.ServicePortConfig{
				{Protocol: "udp", Target: 27015, Published: fmt.Sprintf("%d", udpPort)},
				{Protocol: "tcp", Target: 27020, Published: fmt.Sprintf("%d", tcpPort)},
			},
			func(_, _ string) error { return errors.New("should not be called when --listen is used") },
			nil,
			func() {
				if err := RunCheckReachability(core.CurrentAppDeps(), []string{"--instance", "asa1", "--listen", "--from", "local"}); err != nil {
					t.Fatalf("RunCheckReachability: %v", err)
				}
			},
		)
	})
	if !strings.Contains(out, "OPEN") {
		t.Fatalf("expected OPEN with listener active, got: %s", out)
	}
}

func TestRconLocalOnly(t *testing.T) {
	ports := testPorts("27015:27015/udp", "27020:27020/tcp")
	if rconLocalOnly(ports) {
		t.Fatal("expected all-interfaces RCON binding to be reported as remote-reachable")
	}
	for _, ip := range []string{"127.0.0.1", "::1"} {
		ports[1].HostIP = ip
		if !rconLocalOnly(ports) {
			t.Fatalf("expected %s RCON binding to be local-only", ip)
		}
	}
	ports[1].HostIP = "192.168.1.10"
	if rconLocalOnly(ports) {
		t.Fatal("expected LAN-IP RCON binding not to be treated as local-only")
	}
}

func TestRunCheckReachabilityInstanceNotesLocalOnlyRCON(t *testing.T) {
	ports := testPorts("27015:27015/udp", "27020:27020/tcp")
	ports[1].HostIP = "127.0.0.1"
	out := captureStdout(t, func() {
		withInstanceDeps(t, ports, nil, func(d *core.AppDeps) {
			d.HostDeps.FetchText = func(url string) (string, error) {
				switch {
				case strings.HasSuffix(url, "/api/me"):
					return "203.0.113.1", nil
				case strings.Contains(url, "starrupture-utilities.com"):
					return `{"success":true,"result":{"tcp":{"is_open":false,"status":"closed"},"udp":{"is_open":false,"status":"closed"}}}`, nil
				default:
					return "closed", nil
				}
			}
		}, func() {
			if err := RunCheckReachability(core.CurrentAppDeps(), []string{"--instance", "asa1", "--ports", "rcon", "--from", "remote"}); err != nil {
				t.Fatalf("RunCheckReachability: %v", err)
			}
		})
	})
	if !strings.Contains(out, "published on 127.0.0.1 only") || !strings.Contains(out, "enable remote-rcon --instance asa1") {
		t.Fatalf("expected local-only RCON note, got: %s", out)
	}
}
