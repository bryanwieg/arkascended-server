//go:build integration

package sdkbackend_test

import (
	"log/slog"
	"testing"

	"pkg/ark/conformance"
	"pkg/ark/sdkbackend"
)

func TestSDKComposeConformance(t *testing.T) {
	conformance.RequireDocker(t)
	svc, err := sdkbackend.NewSDKCompose(slog.Default())
	if err != nil {
		t.Fatalf("NewSDKCompose: %v", err)
	}
	conformance.TestComposeService(t, svc)
}
