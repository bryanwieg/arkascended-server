//go:build integration

package sdkbackend_test

import (
	"log/slog"
	"testing"

	"pkg/ark/conformance"
	"pkg/ark/sdkbackend"
)

func TestSDKEngineConformance(t *testing.T) {
	conformance.RequireDocker(t)
	engine, err := sdkbackend.NewSDKEngine(slog.Default())
	if err != nil {
		t.Fatalf("NewSDKEngine: %v", err)
	}
	defer engine.Close()
	conformance.TestContainerEngine(t, engine)
}
