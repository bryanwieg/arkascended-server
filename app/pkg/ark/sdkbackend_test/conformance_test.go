//go:build integration

package sdkbackend_test

import (
	"log/slog"
	"os"
	"testing"

	"pkg/ark/conformance"
	"pkg/ark/host"
	"pkg/ark/sdkbackend"
)

func newTestLogger(t *testing.T) *slog.Logger {
	return slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelWarn}))
}

func TestContainerEngine_Podman(t *testing.T) {
	t.Helper()

	// Check if Podman service is available
	socketPath, rootfulDetected := host.HasPodmanService()
	if socketPath == "" {
		if rootfulDetected {
			t.Skip("Rootful Podman detected but rootless Podman not available")
		}
		t.Skip("Podman service not available")
	}
	// The shared suite pings and seeds fixtures through a DOCKER_HOST client; point it at Podman, as app.go does.
	t.Setenv("DOCKER_HOST", socketPath)

	logger := newTestLogger(t)

	eng, err := sdkbackend.NewSDKEngineWithSocket(logger, socketPath)
	if err != nil {
		t.Skipf("Podman SDK engine init failed: %v", err)
	}
	defer eng.Close()

	conformance.TestContainerEngine(t, eng)
}

func TestComposeService_Podman(t *testing.T) {
	t.Helper()

	// Check if Podman service is available
	socketPath, rootfulDetected := host.HasPodmanService()
	if socketPath == "" {
		if rootfulDetected {
			t.Skip("Rootful Podman detected but rootless Podman not available")
		}
		t.Skip("Podman service not available")
	}
	// The shared suite pings and seeds fixtures through a DOCKER_HOST client; point it at Podman, as app.go does.
	t.Setenv("DOCKER_HOST", socketPath)

	logger := newTestLogger(t)

	svc, err := sdkbackend.NewSDKComposeWithSocket(logger, socketPath)
	if err != nil {
		t.Skipf("Podman SDK compose init failed: %v", err)
	}

	conformance.TestComposeService(t, svc)
}
