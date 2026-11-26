package schedule

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"pkg/ark/filelock"
)

// Paths are under RepoDir/.ark-manage; RepoDir is the compose file's directory (ForComposeFile).

// LogPath returns the log file path for a named schedule.
func (s *Service) LogPath(name string) string {
	return filepath.Join(s.RepoDir, ".ark-manage", "logs", "schedules", name+".log")
}

// StatePath returns the state file path for a named schedule.
func (s *Service) StatePath(name string) string {
	return filepath.Join(s.RepoDir, ".ark-manage", "state", "schedules", name+".last.json")
}

// LockPath returns the lock file path for a named schedule.
func (s *Service) LockPath(name string) string {
	return filepath.Join(s.RepoDir, ".ark-manage", "locks", "schedule-"+name+".lock")
}

// AppendScheduleLog appends a timestamped message to the schedule's log file.
// This is a best-effort operation — errors are logged to stderr rather than returned,
// since a log write failure should not prevent the schedule from continuing.
func (s *Service) AppendScheduleLog(name, message string) {
	path := s.LogPath(name)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		fmt.Fprintf(os.Stderr, "warning: failed to create schedule log dir: %v\n", err)
		return
	}
	f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		fmt.Fprintf(os.Stderr, "warning: failed to open schedule log: %v\n", err)
		return
	}
	defer f.Close()
	if _, err := fmt.Fprintln(f, message); err != nil {
		fmt.Fprintf(os.Stderr, "warning: failed to write schedule log: %v\n", err)
	}
}

// WriteRunState persists the run state for a named schedule.
// This is a best-effort operation — errors are logged to stderr rather than returned,
// since a state write failure should not prevent the schedule from continuing.
func (s *Service) WriteRunState(state RunState) {
	path := s.StatePath(state.Name)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		fmt.Fprintf(os.Stderr, "warning: failed to create schedule state dir: %v\n", err)
		return
	}
	data, err := json.MarshalIndent(state, "", "  ")
	if err != nil {
		fmt.Fprintf(os.Stderr, "warning: failed to marshal schedule state: %v\n", err)
		return
	}
	data = append(data, '\n')
	if err := os.WriteFile(path, data, 0o644); err != nil {
		fmt.Fprintf(os.Stderr, "warning: failed to write schedule state: %v\n", err)
	}
}

// LoadRunState reads the last run state for a named schedule. Returns false
// if the state file does not exist or cannot be parsed.
func (s *Service) LoadRunState(name string) (RunState, bool) {
	path := s.StatePath(name)
	data, err := os.ReadFile(path)
	if err != nil {
		return RunState{}, false
	}
	var state RunState
	if err := json.Unmarshal(data, &state); err != nil {
		return RunState{}, false
	}
	if state.Name == "" {
		state.Name = name
	}
	return state, true
}

// AcquireLock takes the kernel lock for the named schedule without waiting.
// The release function unlocks it; the lock file stays.
func (s *Service) AcquireLock(name string) (func(), error) {
	path := s.LockPath(name)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return nil, err
	}
	release, held, err := filelock.TryLock(path)
	if err != nil {
		return nil, err
	}
	if held {
		return nil, fmt.Errorf("Error: schedule lock held: %s", name)
	}
	return release, nil
}

// LockHeld reports whether the named schedule is running; an error counts as not held.
func (s *Service) LockHeld(name string) bool {
	held, err := filelock.Held(s.LockPath(name))
	return err == nil && held
}
