package arktest

import (
	"context"
	"errors"
	"io"
	"strings"
	"sync"
	"time"

	"pkg/ark"
	"pkg/ark/cmdrunner"
)

// MockEngine implements ark.ContainerEngine for testing. Each method delegates
// to a configurable function field. Unconfigured methods panic.
type MockEngine struct {
	InspectStateFn      func(ctx context.Context, container string) (ark.ContainerState, error)
	RemoveContainerFn   func(ctx context.Context, container string) error
	ContainerLogsFn     func(ctx context.Context, container string, opts ark.LogOptions) (io.ReadCloser, error)
	ImageExistsFn       func(ctx context.Context, image string) (bool, error)
	ImageLabelsFn       func(ctx context.Context, image string) (map[string]string, error)
	PullImageFn         func(ctx context.Context, ref string) error
	BuildImageFn        func(ctx context.Context, req ark.BuildRequest) error
	ReadFileFromImageFn func(ctx context.Context, image, path string) (string, error)
	ImageTagsFn         func(ctx context.Context, repo string) (map[string]string, error)
	RemoveImageFn       func(ctx context.Context, ref string) error
	ImagesInUseFn       func(ctx context.Context) (map[string]bool, error)
	PruneDanglingFn     func(ctx context.Context) error
	VolumeMountpointFn  func(ctx context.Context, volume string) (string, bool)
	RemoveVolumeFn      func(ctx context.Context, volume string) error
	CopyFromContainerFn func(ctx context.Context, container, srcPath, dstPath string) error
}

func (m *MockEngine) InspectState(ctx context.Context, container string) (ark.ContainerState, error) {
	if m.InspectStateFn != nil {
		return m.InspectStateFn(ctx, container)
	}
	return ark.ContainerState{}, nil
}

func (m *MockEngine) RemoveContainer(ctx context.Context, container string) error {
	if m.RemoveContainerFn != nil {
		return m.RemoveContainerFn(ctx, container)
	}
	return nil
}

func (m *MockEngine) ContainerLogs(ctx context.Context, container string, opts ark.LogOptions) (io.ReadCloser, error) {
	if m.ContainerLogsFn != nil {
		return m.ContainerLogsFn(ctx, container, opts)
	}
	return io.NopCloser(strings.NewReader("")), nil
}

// PullImage fails unless mocked, so tests that don't mock it take the local-build path.
func (m *MockEngine) PullImage(ctx context.Context, ref string) error {
	if m.PullImageFn != nil {
		return m.PullImageFn(ctx, ref)
	}
	return errors.New("pull not mocked")
}

func (m *MockEngine) ImageExists(ctx context.Context, image string) (bool, error) {
	if m.ImageExistsFn != nil {
		return m.ImageExistsFn(ctx, image)
	}
	return false, nil
}

func (m *MockEngine) ImageLabels(ctx context.Context, image string) (map[string]string, error) {
	if m.ImageLabelsFn != nil {
		return m.ImageLabelsFn(ctx, image)
	}
	return map[string]string{}, nil
}

func (m *MockEngine) BuildImage(ctx context.Context, req ark.BuildRequest) error {
	if m.BuildImageFn != nil {
		return m.BuildImageFn(ctx, req)
	}
	return nil
}

func (m *MockEngine) ReadFileFromImage(ctx context.Context, image, path string) (string, error) {
	if m.ReadFileFromImageFn != nil {
		return m.ReadFileFromImageFn(ctx, image, path)
	}
	return "", nil
}

func (m *MockEngine) ImageTags(ctx context.Context, repo string) (map[string]string, error) {
	if m.ImageTagsFn != nil {
		return m.ImageTagsFn(ctx, repo)
	}
	return map[string]string{}, nil
}

func (m *MockEngine) RemoveImage(ctx context.Context, ref string) error {
	if m.RemoveImageFn != nil {
		return m.RemoveImageFn(ctx, ref)
	}
	return nil
}

func (m *MockEngine) ImagesInUse(ctx context.Context) (map[string]bool, error) {
	if m.ImagesInUseFn != nil {
		return m.ImagesInUseFn(ctx)
	}
	return map[string]bool{}, nil
}

func (m *MockEngine) PruneDangling(ctx context.Context) error {
	if m.PruneDanglingFn != nil {
		return m.PruneDanglingFn(ctx)
	}
	return nil
}

func (m *MockEngine) VolumeMountpoint(ctx context.Context, volume string) (string, bool) {
	if m.VolumeMountpointFn != nil {
		return m.VolumeMountpointFn(ctx, volume)
	}
	return "", false
}

func (m *MockEngine) RemoveVolume(ctx context.Context, volume string) error {
	if m.RemoveVolumeFn != nil {
		return m.RemoveVolumeFn(ctx, volume)
	}
	return nil
}

func (m *MockEngine) CopyFromContainer(ctx context.Context, container, srcPath, dstPath string) error {
	if m.CopyFromContainerFn != nil {
		return m.CopyFromContainerFn(ctx, container, srcPath, dstPath)
	}
	return nil
}

// MockCompose implements ark.ComposeService for testing.
type MockCompose struct {
	LoadProjectFn          func(ctx context.Context, composeFile string) (*ark.ComposeProject, error)
	SaveProjectFn          func(ctx context.Context, composeFile string, project *ark.ComposeProject) error
	UpFn                   func(ctx context.Context, composeFile string, services []string, opts ark.UpOptions) error
	StopFn                 func(ctx context.Context, composeFile string, services []string, timeout *time.Duration) error
	KillFn                 func(ctx context.Context, composeFile string, services []string, signal string) error
	DownFn                 func(ctx context.Context, composeFile string) error
	RemoveFn               func(ctx context.Context, composeFile string, services []string) error
	CreateFn               func(ctx context.Context, composeFile string, services []string) error
	RunningServicesFn      func(ctx context.Context, composeFile string) (map[string]struct{}, error)
	ExitedServicesFn       func(ctx context.Context, composeFile string) (map[string]struct{}, error)
	AllServicesFn          func(ctx context.Context, composeFile string) (map[string]struct{}, error)
	ExecFn                 func(ctx context.Context, composeFile, service string, command []string, opts ark.ExecOptions) (ark.ExecResult, error)
	ExecInteractiveFn      func(ctx context.Context, composeFile, service string, command []string) error
	RunOneOffFn            func(ctx context.Context, composeFile, service string, command []string, opts ark.RunOptions) (ark.ExecResult, error)
	RunOneOffDetachedFn    func(ctx context.Context, composeFile, service, containerName string, command []string, opts ark.RunOptions) error
	RunOneOffInteractiveFn func(ctx context.Context, composeFile, service string, command []string, opts ark.RunOptions) error
	LogsFn                 func(ctx context.Context, composeFile, service string, opts ark.LogOptions) (io.ReadCloser, error)
	CopyFromServiceFn      func(ctx context.Context, composeFile, service, srcPath, dstPath string) error
}

func (m *MockCompose) LoadProject(ctx context.Context, composeFile string) (*ark.ComposeProject, error) {
	if m.LoadProjectFn != nil {
		return m.LoadProjectFn(ctx, composeFile)
	}
	return &ark.ComposeProject{}, nil
}

func (m *MockCompose) SaveProject(ctx context.Context, composeFile string, project *ark.ComposeProject) error {
	if m.SaveProjectFn != nil {
		return m.SaveProjectFn(ctx, composeFile, project)
	}
	return nil
}

func (m *MockCompose) Up(ctx context.Context, composeFile string, services []string, opts ark.UpOptions) error {
	if m.UpFn != nil {
		return m.UpFn(ctx, composeFile, services, opts)
	}
	return nil
}

func (m *MockCompose) Stop(ctx context.Context, composeFile string, services []string, timeout *time.Duration) error {
	if m.StopFn != nil {
		return m.StopFn(ctx, composeFile, services, timeout)
	}
	return nil
}

func (m *MockCompose) Kill(ctx context.Context, composeFile string, services []string, signal string) error {
	if m.KillFn != nil {
		return m.KillFn(ctx, composeFile, services, signal)
	}
	return nil
}

func (m *MockCompose) Down(ctx context.Context, composeFile string) error {
	if m.DownFn != nil {
		return m.DownFn(ctx, composeFile)
	}
	return nil
}

func (m *MockCompose) Remove(ctx context.Context, composeFile string, services []string) error {
	if m.RemoveFn != nil {
		return m.RemoveFn(ctx, composeFile, services)
	}
	return nil
}

func (m *MockCompose) Create(ctx context.Context, composeFile string, services []string) error {
	if m.CreateFn != nil {
		return m.CreateFn(ctx, composeFile, services)
	}
	return nil
}

func (m *MockCompose) RunningServices(ctx context.Context, composeFile string) (map[string]struct{}, error) {
	if m.RunningServicesFn != nil {
		return m.RunningServicesFn(ctx, composeFile)
	}
	return map[string]struct{}{}, nil
}

func (m *MockCompose) ExitedServices(ctx context.Context, composeFile string) (map[string]struct{}, error) {
	if m.ExitedServicesFn != nil {
		return m.ExitedServicesFn(ctx, composeFile)
	}
	return map[string]struct{}{}, nil
}

func (m *MockCompose) AllServices(ctx context.Context, composeFile string) (map[string]struct{}, error) {
	if m.AllServicesFn != nil {
		return m.AllServicesFn(ctx, composeFile)
	}
	return map[string]struct{}{}, nil
}

func (m *MockCompose) Exec(ctx context.Context, composeFile, service string, command []string, opts ark.ExecOptions) (ark.ExecResult, error) {
	if m.ExecFn != nil {
		return m.ExecFn(ctx, composeFile, service, command, opts)
	}
	return ark.ExecResult{}, nil
}

func (m *MockCompose) ExecInteractive(ctx context.Context, composeFile, service string, command []string) error {
	if m.ExecInteractiveFn != nil {
		return m.ExecInteractiveFn(ctx, composeFile, service, command)
	}
	return nil
}

func (m *MockCompose) RunOneOff(ctx context.Context, composeFile, service string, command []string, opts ark.RunOptions) (ark.ExecResult, error) {
	if m.RunOneOffFn != nil {
		return m.RunOneOffFn(ctx, composeFile, service, command, opts)
	}
	return ark.ExecResult{}, nil
}

func (m *MockCompose) RunOneOffDetached(ctx context.Context, composeFile, service, containerName string, command []string, opts ark.RunOptions) error {
	if m.RunOneOffDetachedFn != nil {
		return m.RunOneOffDetachedFn(ctx, composeFile, service, containerName, command, opts)
	}
	return nil
}

func (m *MockCompose) RunOneOffInteractive(ctx context.Context, composeFile, service string, command []string, opts ark.RunOptions) error {
	if m.RunOneOffInteractiveFn != nil {
		return m.RunOneOffInteractiveFn(ctx, composeFile, service, command, opts)
	}
	return nil
}

func (m *MockCompose) Logs(ctx context.Context, composeFile, service string, opts ark.LogOptions) (io.ReadCloser, error) {
	if m.LogsFn != nil {
		return m.LogsFn(ctx, composeFile, service, opts)
	}
	return io.NopCloser(strings.NewReader("")), nil
}

func (m *MockCompose) CopyFromService(ctx context.Context, composeFile, service, srcPath, dstPath string) error {
	if m.CopyFromServiceFn != nil {
		return m.CopyFromServiceFn(ctx, composeFile, service, srcPath, dstPath)
	}
	return nil
}

// Compile-time interface checks.
var (
	_ ark.ContainerEngine = (*MockEngine)(nil)
	_ ark.ComposeService  = (*MockCompose)(nil)
	_ cmdrunner.Runner    = (*MockCommandRunner)(nil)
)

// MockCommandRunner implements cmdrunner.Runner for testing.
type MockCommandRunner struct {
	mu               sync.Mutex
	RunFn            func(ctx context.Context, cmd []string, opts cmdrunner.RunOptions) (cmdrunner.RunResult, error)
	RunInteractiveFn func(ctx context.Context, cmd []string) error
	Calls            []CallRecord
}

// CallRecord records a single command invocation for later inspection.
type CallRecord struct {
	Cmd  []string
	Opts cmdrunner.RunOptions
}

func (m *MockCommandRunner) Run(ctx context.Context, cmd []string, opts cmdrunner.RunOptions) (cmdrunner.RunResult, error) {
	m.mu.Lock()
	m.Calls = append(m.Calls, CallRecord{Cmd: cmd, Opts: opts})
	m.mu.Unlock()
	if m.RunFn != nil {
		return m.RunFn(ctx, cmd, opts)
	}
	return cmdrunner.RunResult{}, nil
}

func (m *MockCommandRunner) RunInteractive(ctx context.Context, cmd []string) error {
	if m.RunInteractiveFn != nil {
		return m.RunInteractiveFn(ctx, cmd)
	}
	return nil
}
