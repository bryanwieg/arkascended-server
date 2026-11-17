package compose

import (
	"ark-cli/internal/core"

	"pkg/ark/lifecycle"
)

func runDownRequest(req downCommandRequest) error {
	return NewComposeLifecycleCommandService(core.CurrentAppDeps()).runDownRequest(req)
}

// RunDownRequest executes a down command request. Used by teardown and integration tests.
func RunDownRequest(req DownCommandRequest) error {
	return runDownRequest(req)
}

func (s ComposeLifecycleCommandService) runDownRequest(req downCommandRequest) error {
	return s.runWithDeps(func() error {
		return s.runDownRequestImpl(req)
	})
}

func (s ComposeLifecycleCommandService) runDownRequestImpl(req downCommandRequest) error {
	composeFile, _, _, err := ResolveDockerComposeServiceTargets(core.CurrentAppDeps(),
		req.ComposeFileFlag,
		req.Instance,
		"Error: compose config has no services to down.",
	)
	if err != nil {
		return err
	}

	d := core.CurrentAppDeps()
	service := req.Instance
	if service == "all" {
		service = "" // lifecycle.Down treats empty service as "down entire project"
	}
	if err := d.Lifecycle.Down(d.Context(), lifecycle.DownRequest{
		ComposeFile: composeFile,
		Service:     service,
		Timeout:     req.Timeout,
		Kill:        req.Kill,
	}); err != nil {
		return err
	}
	core.Infof("Service is down: %s\n", req.Instance)
	return nil
}
