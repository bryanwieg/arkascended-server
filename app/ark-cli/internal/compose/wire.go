package compose

import (
	"ark-cli/internal/core"

	"pkg/ark/runtime"
)

// WireDefaults sets CLI-orchestration function seams on deps.
// This avoids package main needing to reference individual implementation
// functions, keeping them unexported in the compose package.
func WireDefaults(deps *core.AppDeps) {
	deps.Runtime.RunCommand = core.RunCommandImpl
	deps.Runtime.LookPath = core.LookPathImpl
	deps.Runtime.RconEnvArgs = RconEnvArgsImpl
	deps.RestartStartInstance = func(req runtime.StartCommandRequest) error {
		return RunStartRequest(core.CurrentAppDeps(), req)
	}
	deps.RestartStopInstance = runStopRequest
	deps.RunningServiceSnapshot = func(cf string) (map[string]struct{}, error) {
		return RunningServiceSnapshotImpl(core.CurrentAppDeps(), cf)
	}
}
