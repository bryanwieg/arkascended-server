package testutil

import (
	"context"
	"time"

	"pkg/ark/health"
)

// MockHealthChecker implements health.InstanceHealthChecker for tests.
type MockHealthChecker struct {
	IsStartedFn      func(ctx context.Context, composeFile, service string, since *time.Time) (bool, error)
	IsReadyFn        func(ctx context.Context, composeFile, service string, since *time.Time) (bool, error)
	WaitForStartedFn func(ctx context.Context, composeFile, service string, opts health.WaitOptions) error
	WaitForReadyFn   func(ctx context.Context, composeFile, service string, opts health.WaitOptions) error
	WaitForStoppedFn func(ctx context.Context, composeFile, service string, opts health.WaitOptions) error
}

func NewMockHealthChecker() *MockHealthChecker {
	return &MockHealthChecker{}
}

func (m *MockHealthChecker) IsStarted(ctx context.Context, composeFile, service string, since *time.Time) (bool, error) {
	if m.IsStartedFn != nil {
		return m.IsStartedFn(ctx, composeFile, service, since)
	}
	return true, nil
}

func (m *MockHealthChecker) IsReady(ctx context.Context, composeFile, service string, since *time.Time) (bool, error) {
	if m.IsReadyFn != nil {
		return m.IsReadyFn(ctx, composeFile, service, since)
	}
	return true, nil
}

func (m *MockHealthChecker) WaitForStarted(ctx context.Context, composeFile, service string, opts health.WaitOptions) error {
	if m.WaitForStartedFn != nil {
		return m.WaitForStartedFn(ctx, composeFile, service, opts)
	}
	return nil
}

func (m *MockHealthChecker) WaitForReady(ctx context.Context, composeFile, service string, opts health.WaitOptions) error {
	if m.WaitForReadyFn != nil {
		return m.WaitForReadyFn(ctx, composeFile, service, opts)
	}
	return nil
}

func (m *MockHealthChecker) WaitForStopped(ctx context.Context, composeFile, service string, opts health.WaitOptions) error {
	if m.WaitForStoppedFn != nil {
		return m.WaitForStoppedFn(ctx, composeFile, service, opts)
	}
	return nil
}
