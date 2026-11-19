package lifecycle

import (
	"context"
	"errors"
	"log/slog"
	"testing"
	"time"

	"pkg/ark"
	"pkg/ark/arktest"
	"pkg/ark/health"
)

func testLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(nil, nil))
}

// mockHealth implements HealthChecker for lifecycle tests.
type mockHealth struct {
	waitReadyFn func(ctx context.Context, composeFile, service string, opts health.WaitOptions) error
}

func (m *mockHealth) WaitForReady(ctx context.Context, composeFile, service string, opts health.WaitOptions) error {
	if m.waitReadyFn != nil {
		return m.waitReadyFn(ctx, composeFile, service, opts)
	}
	return nil
}

func newTestService(compose ark.ComposeService, hc HealthChecker) *Service {
	return New(compose, hc, testLogger())
}

func TestStartCallsComposeUp(t *testing.T) {
	var gotServices []string
	var gotPolicy string
	mock := &arktest.MockCompose{
		UpFn: func(_ context.Context, _ string, services []string, opts ark.UpOptions) error {
			gotServices = services
			gotPolicy = opts.PullPolicy
			return nil
		},
	}
	svc := newTestService(mock, &mockHealth{})

	result, err := svc.Start(context.Background(), StartRequest{
		ComposeFile: "docker-compose.yaml",
		Services:    []string{"a", "b"},
		PullPolicy:  "missing",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(gotServices) != 2 || gotServices[0] != "a" || gotServices[1] != "b" {
		t.Fatalf("unexpected services: %v", gotServices)
	}
	if gotPolicy != "missing" {
		t.Fatalf("unexpected pull policy: %s", gotPolicy)
	}
	if len(result.Started) != 2 {
		t.Fatalf("expected 2 started, got %d", len(result.Started))
	}
}

func TestStartUpError(t *testing.T) {
	mock := &arktest.MockCompose{
		UpFn: func(_ context.Context, _ string, _ []string, _ ark.UpOptions) error {
			return errors.New("up failed")
		},
	}
	svc := newTestService(mock, &mockHealth{})

	_, err := svc.Start(context.Background(), StartRequest{
		ComposeFile: "docker-compose.yaml",
		Services:    []string{"a"},
	})
	if err == nil || err.Error() != "compose up: up failed" {
		t.Fatalf("expected wrapped up error, got: %v", err)
	}
}

func TestStartWaitsForReady(t *testing.T) {
	var waitedServices []string
	mock := &arktest.MockCompose{
		RunningServicesFn: func(_ context.Context, _ string) (map[string]struct{}, error) {
			return map[string]struct{}{}, nil // none pre-running
		},
	}
	hc := &mockHealth{
		waitReadyFn: func(_ context.Context, _, service string, _ health.WaitOptions) error {
			waitedServices = append(waitedServices, service)
			return nil
		},
	}
	svc := newTestService(mock, hc)

	_, err := svc.Start(context.Background(), StartRequest{
		ComposeFile:  "docker-compose.yaml",
		Services:     []string{"x", "y"},
		WaitForReady: true,
		WaitTimeout:  30 * time.Second,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(waitedServices) != 2 || waitedServices[0] != "x" || waitedServices[1] != "y" {
		t.Fatalf("expected wait for x,y, got: %v", waitedServices)
	}
}

func TestStartWaitReadyError(t *testing.T) {
	mock := &arktest.MockCompose{
		RunningServicesFn: func(_ context.Context, _ string) (map[string]struct{}, error) {
			return map[string]struct{}{}, nil
		},
	}
	hc := &mockHealth{
		waitReadyFn: func(_ context.Context, _, service string, _ health.WaitOptions) error {
			if service == "b" {
				return errors.New("b not ready")
			}
			return nil
		},
	}
	svc := newTestService(mock, hc)

	result, err := svc.Start(context.Background(), StartRequest{
		ComposeFile:  "docker-compose.yaml",
		Services:     []string{"a", "b"},
		WaitForReady: true,
	})
	if err == nil {
		t.Fatal("expected readiness error")
	}
	// Result should still report what was started.
	if len(result.Started) != 2 {
		t.Fatalf("expected started list, got %d", len(result.Started))
	}
}

func TestStartSkipsWaitWhenNotRequested(t *testing.T) {
	hc := &mockHealth{
		waitReadyFn: func(_ context.Context, _, _ string, _ health.WaitOptions) error {
			t.Fatal("should not wait when WaitForReady is false")
			return nil
		},
	}
	svc := newTestService(&arktest.MockCompose{}, hc)

	_, err := svc.Start(context.Background(), StartRequest{
		ComposeFile:  "docker-compose.yaml",
		Services:     []string{"a"},
		WaitForReady: false,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestStopGraceful(t *testing.T) {
	var gotTimeout *time.Duration
	mock := &arktest.MockCompose{
		StopFn: func(_ context.Context, _ string, _ []string, timeout *time.Duration) error {
			gotTimeout = timeout
			return nil
		},
	}
	svc := newTestService(mock, &mockHealth{})

	err := svc.Stop(context.Background(), StopRequest{
		ComposeFile: "docker-compose.yaml",
		Services:    []string{"a"},
		Timeout:     30 * time.Second,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if gotTimeout == nil || *gotTimeout != 30*time.Second {
		t.Fatalf("expected 30s timeout, got: %v", gotTimeout)
	}
}

func TestStopDefaultTimeout(t *testing.T) {
	var gotTimeout *time.Duration
	mock := &arktest.MockCompose{
		StopFn: func(_ context.Context, _ string, _ []string, timeout *time.Duration) error {
			gotTimeout = timeout
			return nil
		},
	}
	svc := newTestService(mock, &mockHealth{})

	err := svc.Stop(context.Background(), StopRequest{
		ComposeFile: "docker-compose.yaml",
		Services:    []string{"a"},
		Timeout:     -1,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if gotTimeout != nil {
		t.Fatalf("expected nil timeout (compose stop_grace_period), got: %v", *gotTimeout)
	}
}

func TestStopKill(t *testing.T) {
	var gotSignal string
	mock := &arktest.MockCompose{
		KillFn: func(_ context.Context, _ string, _ []string, signal string) error {
			gotSignal = signal
			return nil
		},
	}
	svc := newTestService(mock, &mockHealth{})

	err := svc.Stop(context.Background(), StopRequest{
		ComposeFile: "docker-compose.yaml",
		Services:    []string{"a"},
		Kill:        true,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if gotSignal != "SIGKILL" {
		t.Fatalf("expected SIGKILL, got: %s", gotSignal)
	}
}

func TestDownAllStopsRunningThenDown(t *testing.T) {
	var order []string
	mock := &arktest.MockCompose{
		RunningServicesFn: func(_ context.Context, _ string) (map[string]struct{}, error) {
			return map[string]struct{}{"a": {}}, nil
		},
		StopFn: func(_ context.Context, _ string, services []string, _ *time.Duration) error {
			for _, s := range services {
				order = append(order, "stop:"+s)
			}
			return nil
		},
		DownFn: func(_ context.Context, _ string) error {
			order = append(order, "down")
			return nil
		},
	}
	svc := newTestService(mock, &mockHealth{})

	err := svc.Down(context.Background(), DownRequest{
		ComposeFile: "docker-compose.yaml",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(order) != 2 || order[0] != "stop:a" || order[1] != "down" {
		t.Fatalf("expected [stop:a, down], got: %v", order)
	}
}

func TestDownAllErrorOnDown(t *testing.T) {
	mock := &arktest.MockCompose{
		RunningServicesFn: func(_ context.Context, _ string) (map[string]struct{}, error) {
			return map[string]struct{}{}, nil
		},
		DownFn: func(_ context.Context, _ string) error {
			return errors.New("down failed")
		},
	}
	svc := newTestService(mock, &mockHealth{})

	err := svc.Down(context.Background(), DownRequest{ComposeFile: "docker-compose.yaml"})
	if err == nil || err.Error() != "down failed" {
		t.Fatalf("expected down error, got: %v", err)
	}
}

func TestDownServiceStopsAndRemoves(t *testing.T) {
	var order []string
	mock := &arktest.MockCompose{
		RunningServicesFn: func(_ context.Context, _ string) (map[string]struct{}, error) {
			return map[string]struct{}{"a": {}}, nil
		},
		AllServicesFn: func(_ context.Context, _ string) (map[string]struct{}, error) {
			return map[string]struct{}{"a": {}}, nil
		},
		StopFn: func(_ context.Context, _ string, services []string, _ *time.Duration) error {
			order = append(order, "stop:"+services[0])
			return nil
		},
		RemoveFn: func(_ context.Context, _ string, services []string) error {
			order = append(order, "remove:"+services[0])
			return nil
		},
	}
	svc := newTestService(mock, &mockHealth{})

	err := svc.Down(context.Background(), DownRequest{
		ComposeFile: "docker-compose.yaml",
		Service:     "a",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(order) != 2 || order[0] != "stop:a" || order[1] != "remove:a" {
		t.Fatalf("expected [stop:a, remove:a], got: %v", order)
	}
}

func TestDownServiceNotRunningSkipsStop(t *testing.T) {
	var order []string
	mock := &arktest.MockCompose{
		RunningServicesFn: func(_ context.Context, _ string) (map[string]struct{}, error) {
			return map[string]struct{}{}, nil // not running
		},
		AllServicesFn: func(_ context.Context, _ string) (map[string]struct{}, error) {
			return map[string]struct{}{"a": {}}, nil
		},
		RemoveFn: func(_ context.Context, _ string, services []string) error {
			order = append(order, "remove:"+services[0])
			return nil
		},
	}
	svc := newTestService(mock, &mockHealth{})

	err := svc.Down(context.Background(), DownRequest{
		ComposeFile: "docker-compose.yaml",
		Service:     "a",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(order) != 1 || order[0] != "remove:a" {
		t.Fatalf("expected [remove:a], got: %v", order)
	}
}

func TestDownServiceAlreadyGoneIsNoop(t *testing.T) {
	mock := &arktest.MockCompose{
		RunningServicesFn: func(_ context.Context, _ string) (map[string]struct{}, error) {
			return map[string]struct{}{}, nil
		},
		AllServicesFn: func(_ context.Context, _ string) (map[string]struct{}, error) {
			return map[string]struct{}{}, nil // service doesn't exist
		},
	}
	svc := newTestService(mock, &mockHealth{})

	err := svc.Down(context.Background(), DownRequest{
		ComposeFile: "docker-compose.yaml",
		Service:     "gone",
	})
	if err != nil {
		t.Fatalf("expected no error for missing service, got: %v", err)
	}
}

func TestDownServiceRemoveError(t *testing.T) {
	mock := &arktest.MockCompose{
		RunningServicesFn: func(_ context.Context, _ string) (map[string]struct{}, error) {
			return map[string]struct{}{}, nil
		},
		AllServicesFn: func(_ context.Context, _ string) (map[string]struct{}, error) {
			return map[string]struct{}{"a": {}}, nil
		},
		RemoveFn: func(_ context.Context, _ string, _ []string) error {
			return errors.New("rm failed")
		},
	}
	svc := newTestService(mock, &mockHealth{})

	err := svc.Down(context.Background(), DownRequest{
		ComposeFile: "docker-compose.yaml",
		Service:     "a",
	})
	if err == nil || err.Error() != "rm failed" {
		t.Fatalf("expected rm error, got: %v", err)
	}
}
