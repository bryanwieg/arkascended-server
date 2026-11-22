package runtime

import (
	"io"
	"testing"

	"pkg/ark"
)

func TestEnsureDockerGroupSkipsRootlessDockerHost(t *testing.T) {
	t.Setenv("DOCKER_HOST", "unix:///run/user/1000/docker.sock")
	h := &RuntimeHelper{Runtime: RuntimeDeps{
		ContainerRuntime: RuntimeDocker,
		RunCommand: func(cmd []string, _ ark.CommandOptions) (ark.ExecResult, error) {
			t.Fatalf("unexpected command: %v", cmd)
			return ark.ExecResult{}, nil
		},
	}}
	if err := h.EnsureDockerGroup(io.Discard); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}
