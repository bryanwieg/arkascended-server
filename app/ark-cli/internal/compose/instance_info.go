package compose

import (
	"ark-cli/internal/core"

	"pkg/ark"
	"pkg/ark/runtime"
)

type InstanceSummary = runtime.InstanceSummary

func RunningServiceSnapshotImpl(d *core.AppDeps, composeFile string) (map[string]struct{}, error) {
	return d.RuntimeHelper.RunningServiceSnapshot(composeFile)
}

func DoRunningServiceSnapshot(d *core.AppDeps, composeFile string) (map[string]struct{}, error) {
	if dep := d.RunningServiceSnapshot; dep != nil {
		return dep(composeFile)
	}
	return RunningServiceSnapshotImpl(d, composeFile)
}

func ListInstanceSummaries(d *core.AppDeps, composeFile string, project *ark.ComposeProject) []InstanceSummary {
	services := project.ServiceNames()
	out := make([]InstanceSummary, 0, len(services))
	runningSnapshot, err := DoRunningServiceSnapshot(d, composeFile)
	if err != nil {
		runningSnapshot = nil
	}
	for _, name := range services {
		out = append(out, d.RuntimeHelper.BuildInstanceSummaryWithRunning(composeFile, name, project.Project.Services[name], runningSnapshot))
	}
	return out
}
