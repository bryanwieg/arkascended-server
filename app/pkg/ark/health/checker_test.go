package health

import (
	"context"
	"io"
	"log/slog"
	"strings"
	"testing"
	"time"

	"pkg/ark"
	"pkg/ark/arktest"
)

func TestLogsContainMarker(t *testing.T) {
	mock := &arktest.MockCompose{
		LogsFn: func(_ context.Context, _, _ string, _ ark.LogOptions) (io.ReadCloser, error) {
			return io.NopCloser(strings.NewReader("line 1\n" + StartedMarker + "\nline 3\n")), nil
		},
	}
	c := New(mock, nil, slog.Default())

	found, err := c.LogsContainMarker(context.Background(), "dc.yaml", "svc1", StartedMarker, nil)
	if err != nil {
		t.Fatal(err)
	}
	if !found {
		t.Fatal("expected marker found")
	}

	found, err = c.LogsContainMarker(context.Background(), "dc.yaml", "svc1", ReadyMarker, nil)
	if err != nil {
		t.Fatal(err)
	}
	if found {
		t.Fatal("expected marker not found")
	}
}

func TestIsStarted(t *testing.T) {
	mock := &arktest.MockCompose{
		LogsFn: func(_ context.Context, _, _ string, _ ark.LogOptions) (io.ReadCloser, error) {
			return io.NopCloser(strings.NewReader(StartedMarker)), nil
		},
	}
	c := New(mock, nil, slog.Default())

	ok, err := c.IsStarted(context.Background(), "dc.yaml", "svc1", nil)
	if err != nil {
		t.Fatal(err)
	}
	if !ok {
		t.Fatal("expected started")
	}
}

func TestIsReady(t *testing.T) {
	mock := &arktest.MockCompose{
		LogsFn: func(_ context.Context, _, _ string, _ ark.LogOptions) (io.ReadCloser, error) {
			return io.NopCloser(strings.NewReader(ReadyMarker)), nil
		},
	}
	c := New(mock, nil, slog.Default())

	ok, err := c.IsReady(context.Background(), "dc.yaml", "svc1", nil)
	if err != nil {
		t.Fatal(err)
	}
	if !ok {
		t.Fatal("expected ready")
	}
}

func TestIsRunning(t *testing.T) {
	mock := &arktest.MockCompose{
		RunningServicesFn: func(_ context.Context, _ string) (map[string]struct{}, error) {
			return map[string]struct{}{"svc1": {}}, nil
		},
	}
	c := New(mock, nil, slog.Default())

	ok, err := c.IsRunning(context.Background(), "dc.yaml", "svc1")
	if err != nil {
		t.Fatal(err)
	}
	if !ok {
		t.Fatal("expected running")
	}

	ok, err = c.IsRunning(context.Background(), "dc.yaml", "svc2")
	if err != nil {
		t.Fatal(err)
	}
	if ok {
		t.Fatal("expected not running")
	}
}

func TestCheckNotExited(t *testing.T) {
	mock := &arktest.MockCompose{
		ExitedServicesFn: func(_ context.Context, _ string) (map[string]struct{}, error) {
			return map[string]struct{}{"svc1": {}}, nil
		},
	}
	c := New(mock, nil, slog.Default())

	err := c.CheckNotExited(context.Background(), "dc.yaml", "svc1")
	if err == nil {
		t.Fatal("expected error for exited service")
	}

	err = c.CheckNotExited(context.Background(), "dc.yaml", "svc2")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestWaitForStarted(t *testing.T) {
	calls := 0
	mock := &arktest.MockCompose{
		RunningServicesFn: func(_ context.Context, _ string) (map[string]struct{}, error) {
			return map[string]struct{}{"svc1": {}}, nil
		},
		LogsFn: func(_ context.Context, _, _ string, _ ark.LogOptions) (io.ReadCloser, error) {
			calls++
			if calls >= 2 {
				return io.NopCloser(strings.NewReader(StartedMarker)), nil
			}
			return io.NopCloser(strings.NewReader("not yet")), nil
		},
	}
	c := New(mock, nil, slog.Default())

	err := c.WaitForStarted(context.Background(), "dc.yaml", "svc1", WaitOptions{
		Timeout: 5 * time.Second, Interval: 10 * time.Millisecond,
	})
	if err != nil {
		t.Fatal(err)
	}
	if calls < 2 {
		t.Fatalf("expected at least 2 log calls, got %d", calls)
	}
}

func TestWaitForStopped(t *testing.T) {
	calls := 0
	mock := &arktest.MockCompose{
		RunningServicesFn: func(_ context.Context, _ string) (map[string]struct{}, error) {
			calls++
			if calls >= 2 {
				return map[string]struct{}{}, nil
			}
			return map[string]struct{}{"svc1": {}}, nil
		},
	}
	c := New(mock, nil, slog.Default())

	err := c.WaitForStopped(context.Background(), "dc.yaml", "svc1", WaitOptions{
		Timeout: 5 * time.Second, Interval: 10 * time.Millisecond,
	})
	if err != nil {
		t.Fatal(err)
	}
}

func TestWaitForStartedTimeout(t *testing.T) {
	mock := &arktest.MockCompose{
		RunningServicesFn: func(_ context.Context, _ string) (map[string]struct{}, error) {
			return map[string]struct{}{"svc1": {}}, nil
		},
		LogsFn: func(_ context.Context, _, _ string, _ ark.LogOptions) (io.ReadCloser, error) {
			return io.NopCloser(strings.NewReader("not yet")), nil
		},
	}
	c := New(mock, nil, slog.Default())

	err := c.WaitForStarted(context.Background(), "dc.yaml", "svc1", WaitOptions{
		Timeout: 50 * time.Millisecond, Interval: 10 * time.Millisecond,
	})
	if err == nil {
		t.Fatal("expected timeout error")
	}
	if !strings.Contains(err.Error(), "timed out") {
		t.Fatalf("expected timeout message, got: %v", err)
	}
}

func TestWaitForStartedExitedEarly(t *testing.T) {
	mock := &arktest.MockCompose{
		RunningServicesFn: func(_ context.Context, _ string) (map[string]struct{}, error) {
			return map[string]struct{}{}, nil // not running
		},
		ExitedServicesFn: func(_ context.Context, _ string) (map[string]struct{}, error) {
			return map[string]struct{}{"svc1": {}}, nil // exited
		},
	}
	c := New(mock, nil, slog.Default())

	err := c.WaitForStarted(context.Background(), "dc.yaml", "svc1", WaitOptions{
		Timeout: 1 * time.Second, Interval: 10 * time.Millisecond,
	})
	if err == nil {
		t.Fatal("expected error for exited service")
	}
	if !strings.Contains(err.Error(), "exited prematurely") {
		t.Fatalf("expected premature exit error, got: %v", err)
	}
}

// With no explicit since, logs are read from the current container start with
// no tail limit, so an old run's marker can't match and a long run can't drop it.
func TestLogsContainMarkerReadsFromCurrentRunStart(t *testing.T) {
	started := time.Date(2026, 11, 18, 12, 0, 0, 0, time.UTC)
	var got ark.LogOptions
	mock := &arktest.MockCompose{
		ServiceContainersFn: func(context.Context, string) (map[string]ark.ServiceContainer, error) {
			return map[string]ark.ServiceContainer{"svc1": {Name: "proj-svc1-1", State: "running"}}, nil
		},
		LogsFn: func(_ context.Context, _, _ string, opts ark.LogOptions) (io.ReadCloser, error) {
			got = opts
			return io.NopCloser(strings.NewReader("")), nil
		},
	}
	engine := &arktest.MockEngine{
		InspectStateFn: func(_ context.Context, name string) (ark.ContainerState, error) {
			if name != "proj-svc1-1" {
				t.Fatalf("inspected %q", name)
			}
			return ark.ContainerState{Running: true, StartedAt: started}, nil
		},
	}
	if _, err := New(mock, engine, slog.Default()).IsReady(context.Background(), "dc.yaml", "svc1", nil); err != nil {
		t.Fatal(err)
	}
	if got.Since == nil || !got.Since.Equal(started) || got.Tail != 0 {
		t.Fatalf("log options = %+v, want Since=%v Tail=0", got, started)
	}
}
