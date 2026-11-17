package listenport

import (
	"fmt"
	"net"
	"strings"
	"syscall"
	"testing"
	"time"

	"ark-cli/internal/core"
	"ark-cli/internal/testutil"
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

func captureStdout(t *testing.T, fn func()) string {
	t.Helper()
	return testutil.CaptureStdout(t, fn)
}

func withPatchedAppDeps(t *testing.T, patch func(*core.AppDeps), fn func()) {
	t.Helper()
	testutil.WithPatchedAppDeps(t, testutil.ComposeOnlyConfig, patch, fn)
}

func probeLocalTCP(port int) error {
	conn, err := net.DialTimeout("tcp", fmt.Sprintf("127.0.0.1:%d", port), 300*time.Millisecond)
	if err != nil {
		return err
	}
	_ = conn.Close()
	return nil
}

// runUntilSIGINT starts RunListenPort on a goroutine, waits until ready reports the listener is
// up, then sends SIGINT and returns RunListenPort's error. The deps patch wraps the whole run, so
// its restore can't overlap the next test, and SIGINT is only sent once the port answers (by then
// RunListenPort has registered its handler; an unhandled SIGINT would kill the test binary).
func runUntilSIGINT(t *testing.T, args []string, ready func() bool) error {
	t.Helper()
	var runErr error
	withPatchedAppDeps(t, nil, func() {
		d := core.CurrentAppDeps()
		done := make(chan error, 1)
		go func() { done <- RunListenPort(d, args) }()

		deadline := time.Now().Add(2 * time.Second)
		for !ready() {
			if time.Now().After(deadline) {
				// Stop RunListenPort before failing: left running, its handler would swallow
				// later tests' SIGINTs and hang them.
				_ = syscall.Kill(syscall.Getpid(), syscall.SIGINT)
				select {
				case <-done:
				case <-time.After(2 * time.Second):
				}
				t.Fatalf("listener for %v did not come up within 2s", args)
			}
			time.Sleep(10 * time.Millisecond)
		}
		if err := syscall.Kill(syscall.Getpid(), syscall.SIGINT); err != nil {
			t.Fatalf("send SIGINT: %v", err)
		}
		runErr = <-done
	})
	return runErr
}

func TestRunListenPortOpensTCPListener(t *testing.T) {
	port := pickFreeTCPPort(t)
	args := []string{"--port", fmt.Sprintf("%d", port), "--protocol", "tcp"}
	if err := runUntilSIGINT(t, args, func() bool { return probeLocalTCP(port) == nil }); err != nil {
		t.Fatalf("RunListenPort returned error: %v", err)
	}
	if probeLocalTCP(port) == nil {
		t.Fatalf("expected TCP listener to be closed after exit on port %d", port)
	}
}

func TestRunListenPortOpensUDPListener(t *testing.T) {
	port := pickFreeUDPPort(t)
	pc, err := net.ListenPacket("udp", "127.0.0.1:0")
	if err != nil {
		skipIfSocketDenied(t, err)
		t.Fatalf("listen udp: %v", err)
	}
	defer pc.Close()

	var replyBuf [2048]byte
	udpReplies := func() bool {
		_ = pc.SetWriteDeadline(time.Now().Add(100 * time.Millisecond))
		if _, err := pc.WriteTo([]byte("probe"), &net.UDPAddr{IP: net.ParseIP("127.0.0.1"), Port: port}); err != nil {
			return false
		}
		_ = pc.SetReadDeadline(time.Now().Add(100 * time.Millisecond))
		n, _, err := pc.ReadFrom(replyBuf[:])
		return err == nil && n > 0
	}
	args := []string{"--port", fmt.Sprintf("%d", port), "--protocol", "udp"}
	if err := runUntilSIGINT(t, args, udpReplies); err != nil {
		t.Fatalf("RunListenPort returned error: %v", err)
	}
}

func TestRunListenPortOpensBothProtocols(t *testing.T) {
	port := pickFreeTCPAndUDPPort(t)
	args := []string{"--port", fmt.Sprintf("%d", port)}
	if err := runUntilSIGINT(t, args, func() bool { return probeLocalTCP(port) == nil }); err != nil {
		t.Fatalf("RunListenPort returned error: %v", err)
	}
}

func TestRunListenPortInvalidPort(t *testing.T) {
	withPatchedAppDeps(t, nil, func() {
		err := RunListenPort(core.CurrentAppDeps(), []string{"--port", "0"})
		if err == nil {
			t.Fatal("expected error for invalid port")
		}
		if !strings.Contains(err.Error(), "required") {
			t.Fatalf("expected invalid port error, got: %v", err)
		}
	})
}

func TestRunListenPortInvalidProtocol(t *testing.T) {
	withPatchedAppDeps(t, nil, func() {
		err := RunListenPort(core.CurrentAppDeps(), []string{"--port", "27015", "--protocol", "icmp"})
		if err == nil {
			t.Fatal("expected error for invalid protocol")
		}
		if !strings.Contains(err.Error(), "--protocol must be one of") {
			t.Fatalf("expected protocol error, got: %v", err)
		}
	})
}
