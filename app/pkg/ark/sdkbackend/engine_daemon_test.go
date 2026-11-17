package sdkbackend

import (
	"strings"
	"testing"
)

func TestValidateDaemonVersionString(t *testing.T) {
	tests := []struct {
		name    string
		version string
		runtime string
		wantErr string // substring to match, empty means nil
	}{
		// Docker version checks
		{"docker 27 fails", "27.0.0", "docker", "docker 28.0+"},
		{"docker 27.5 fails", "27.5.3", "docker", "docker 28.0+"},
		{"docker 28 passes", "28.0.0", "docker", ""},
		{"docker 28.5 passes", "28.5.0", "docker", ""},
		{"docker 28.1 passes", "28.1.2", "docker", ""},
		{"docker 30 passes", "30.0.0", "docker", ""},

		// Podman version checks
		{"podman 4 fails", "4.0.0", "podman", "podman 5.0+"},
		{"podman 4.9 fails", "4.9.3", "podman", "podman 5.0+"},
		{"podman 5.0 passes", "5.0.0", "podman", ""},
		{"podman 5.5 passes", "5.5.1", "podman", ""},
		{"podman 6 passes", "6.0.0", "podman", ""},

		// Default (empty runtime) treated as docker
		{"default runtime 27 fails", "27.0.0", "", "docker 28.0+"},
		{"default runtime 28 passes", "28.0.0", "", ""},

		// Unparseable versions — graceful skip
		{"non-numeric version", "abc", "docker", ""},
		{"empty version", "", "docker", ""},
		{"partial version", "v28", "docker", ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateDaemonVersionString(tt.version, "1.0", tt.runtime)
			if tt.wantErr == "" {
				if err != nil {
					t.Errorf("unexpected error: %v", err)
				}
			} else {
				if err == nil {
					t.Errorf("expected error containing %q, got nil", tt.wantErr)
				} else if !strings.Contains(err.Error(), tt.wantErr) {
					t.Errorf("error %q does not contain %q", err.Error(), tt.wantErr)
				}
			}
		})
	}
}

func TestValidateDaemonVersionStringAPIInError(t *testing.T) {
	err := ValidateDaemonVersionString("27.0.0", "1.48", "docker")
	if err == nil {
		t.Fatal("expected error for docker 27")
	}
	if !strings.Contains(err.Error(), "1.48") {
		t.Errorf("error should include API version, got: %v", err)
	}
}

func TestValidateDaemonVersionStringPodmanAPIInError(t *testing.T) {
	err := ValidateDaemonVersionString("4.9.3", "1.44", "podman")
	if err == nil {
		t.Fatal("expected error for podman 4")
	}
	if !strings.Contains(err.Error(), "1.44") {
		t.Errorf("error should include API version, got: %v", err)
	}
}
