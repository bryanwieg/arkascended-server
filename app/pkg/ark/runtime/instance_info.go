package runtime

import (
	"errors"
	"fmt"
	"strconv"
	"strings"

	"pkg/ark"
	"pkg/ark/composeutil"
	"pkg/ark/host"
	"pkg/ark/schedule"

	"github.com/compose-spec/compose-go/v2/types"
)

// InstanceSummary holds summary information about a compose service instance.
type InstanceSummary struct {
	Name         string
	Status       string
	Image        string
	Map          string
	Session      string
	ASAPort      int
	BackupDir    string
	LatestBackup string
}

// RunningServiceSet returns the set of currently running services in a compose project.
func (h *RuntimeHelper) RunningServiceSet(composeFile string) (map[string]struct{}, error) {
	if h.ComposeService == nil {
		return map[string]struct{}{}, nil
	}
	return h.ComposeService.RunningServices(h.Ctx, composeFile)
}

// ExitedServiceSet returns the set of exited services in a compose project.
func (h *RuntimeHelper) ExitedServiceSet(composeFile string) (map[string]struct{}, error) {
	if h.ComposeService == nil {
		return map[string]struct{}{}, nil
	}
	return h.ComposeService.ExitedServices(h.Ctx, composeFile)
}

// AllServiceSet returns the set of all services in a compose project.
func (h *RuntimeHelper) AllServiceSet(composeFile string) (map[string]struct{}, error) {
	if h.ComposeService == nil {
		return map[string]struct{}{}, nil
	}
	return h.ComposeService.AllServices(h.Ctx, composeFile)
}

// ServiceRunningStatus checks whether a compose service is currently running.
func (h *RuntimeHelper) ServiceRunningStatus(composeFile, service string) (bool, error) {
	services, err := h.RunningServiceSet(composeFile)
	if err != nil {
		return false, err
	}
	return composeHasService(services, service), nil
}

// EnsureServiceRunning returns an error if the given compose service is not running.
func (h *RuntimeHelper) EnsureServiceRunning(composeFile, service string) error {
	isRunning, err := h.ServiceRunningStatus(composeFile, service)
	if err != nil {
		return err
	}
	if !isRunning {
		return fmt.Errorf("Error: service is not running: %s", service)
	}
	return nil
}

// RunningServiceSnapshot returns running services, trimming error prefixes.
func (h *RuntimeHelper) RunningServiceSnapshot(composeFile string) (map[string]struct{}, error) {
	services, err := h.RunningServiceSet(composeFile)
	if err == nil {
		return services, nil
	}
	trimmed := strings.TrimPrefix(err.Error(), "Error: ")
	return nil, errors.New(trimmed)
}

// LoadComposeProject loads a compose file and returns the typed ComposeProject.
func (h *RuntimeHelper) LoadComposeProject(composeFile string) (*ark.ComposeProject, error) {
	if h.ComposeService == nil {
		return nil, fmt.Errorf("Error: composeService not initialized")
	}
	return h.ComposeService.LoadProject(h.Ctx, composeFile)
}

// BuildInstanceSummary builds a summary for a service without a pre-computed running set.
func (h *RuntimeHelper) BuildInstanceSummary(composeFile, name string, service types.ServiceConfig) InstanceSummary {
	return h.BuildInstanceSummaryWithRunning(composeFile, name, service, nil)
}

// BuildInstanceSummaryWithRunning builds a summary for a service, using a pre-computed
// running set if available, or checking running status individually otherwise.
func (h *RuntimeHelper) BuildInstanceSummaryWithRunning(composeFile, name string, service types.ServiceConfig, running map[string]struct{}) InstanceSummary {
	mapName, _ := composeutil.EnvValue(service.Environment, "MAP_NAME")
	sessionName, _ := composeutil.EnvValue(service.Environment, "SESSION_NAME")
	summary := InstanceSummary{
		Name:    name,
		Status:  "stopped",
		Image:   service.Image,
		Map:     mapName,
		Session: sessionName,
	}
	isRunning := false
	if running != nil {
		_, isRunning = running[name]
	} else {
		status, err := h.ServiceRunningStatus(composeFile, name)
		isRunning = err == nil && status
	}
	if isRunning {
		summary.Status = "running"
	}
	if port := ServiceASAPort(service); port > 0 {
		summary.ASAPort = port
	}
	return summary
}

// ListInstanceSummaries returns summaries for all services in a compose project.
func (h *RuntimeHelper) ListInstanceSummaries(composeFile string, project *ark.ComposeProject) []InstanceSummary {
	services := project.ServiceNames()
	out := make([]InstanceSummary, 0, len(services))
	runningSnapshot, err := h.RunningServiceSnapshot(composeFile)
	if err != nil {
		runningSnapshot = nil
	}
	for _, name := range services {
		out = append(out, h.BuildInstanceSummaryWithRunning(composeFile, name, project.Project.Services[name], runningSnapshot))
	}
	return out
}

// ServiceEnvValue extracts an environment variable value from a service config.
func ServiceEnvValue(service types.ServiceConfig, key string) string {
	val, _ := composeutil.EnvValue(service.Environment, key)
	return val
}

// IntOrDash formats an integer as a string, or "-" if zero or negative.
func IntOrDash(value int) string {
	if value <= 0 {
		return "-"
	}
	return strconv.Itoa(value)
}

// FallbackDash returns the value or "-" if it's blank.
func FallbackDash(value string) string {
	if strings.TrimSpace(value) == "" {
		return "-"
	}
	return value
}

// ServiceASAPort resolves the ASA game port for a service.
// Prefers the ark.port.asa label (new scheme); falls back to scanning for the
// legacy fixed container port 27015/udp (old scheme).
func ServiceASAPort(service types.ServiceConfig) int {
	if raw := service.Labels[LabelPortASA]; raw != "" {
		if port, err := strconv.Atoi(raw); err == nil && port > 0 {
			return port
		}
	}
	port, err := host.ResolveUDPHostPort(service.Ports)
	if err != nil {
		return 0
	}
	return port
}

// RCONContainerPort is the port RCON listens on inside the container: the
// service's RCON_PORT (the entrypoint writes it to the INI), else 27020.
func RCONContainerPort(service types.ServiceConfig) int {
	if raw, ok := composeutil.EnvValue(service.Environment, "RCON_PORT"); ok {
		if port, err := strconv.Atoi(strings.TrimSpace(raw)); err == nil && port > 0 && port <= 65535 {
			return port
		}
	}
	return DefaultRCONContainerPort
}

// DefaultRCONContainerPort is the container's RCON port when RCON_PORT is unset.
const DefaultRCONContainerPort = 27020

// InstanceHasEnabledScheduleInFile checks if any backup schedule is enabled for the instance.
func InstanceHasEnabledScheduleInFile(file schedule.ScheduleFile, instance string) bool {
	for _, s := range file.Schedules {
		if s.Enabled && s.Type == "backup" && (s.Instance == instance || s.Instance == "all") {
			return true
		}
	}
	return false
}

// composeHasService checks if a service name exists in a service set.
func composeHasService(services map[string]struct{}, service string) bool {
	_, ok := services[service]
	return ok
}

// LabelPortASA is the label key for the ASA game port.
const LabelPortASA = ark.PortASALabel
