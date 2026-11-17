package update

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// UpdateResult is the outcome of one update attempt.
type UpdateResult string

const (
	ResultSucceeded      UpdateResult = "succeeded"
	ResultRolledBack     UpdateResult = "rolled_back"
	ResultRollbackFailed UpdateResult = "rollback_failed"
	ResultBuildFailed    UpdateResult = "build_failed"
)

// UpdateAttempt records one update of an instance.
type UpdateAttempt struct {
	Time    time.Time    `json:"time"`
	From    string       `json:"from"`
	To      string       `json:"to"`
	Result  UpdateResult `json:"result"`
	Message string       `json:"message,omitempty"`
	Backup  string       `json:"backup,omitempty"`
}

// InstanceState is the update state kept per instance.
type InstanceState struct {
	PreviousImage string         `json:"previous_image,omitempty"`
	LastAttempt   *UpdateAttempt `json:"last_attempt,omitempty"`
}

// StatePath returns <stateDir>/state/updates/<instance>.json.
func StatePath(stateDir, instance string) string {
	return filepath.Join(stateDir, "state", "updates", instance+".json")
}

// LoadState reads an instance's state; a missing file is the zero state.
func LoadState(stateDir, instance string) (InstanceState, error) {
	path := StatePath(stateDir, instance)
	data, err := os.ReadFile(path)
	if errors.Is(err, fs.ErrNotExist) {
		return InstanceState{}, nil
	}
	if err != nil {
		return InstanceState{}, err
	}
	var st InstanceState
	if err := json.Unmarshal(data, &st); err != nil {
		return InstanceState{}, fmt.Errorf("Error: invalid update state %s: %w", path, err)
	}
	return st, nil
}

// SaveState writes an instance's state atomically (temp file + rename).
func SaveState(stateDir, instance string, st InstanceState) error {
	path := StatePath(stateDir, instance)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(st, "", "  ")
	if err != nil {
		return err
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, append(data, '\n'), 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

// LoadAllStates reads every instance's state; a missing directory is an empty map.
func LoadAllStates(stateDir string) (map[string]InstanceState, error) {
	entries, err := os.ReadDir(filepath.Dir(StatePath(stateDir, "x")))
	if errors.Is(err, fs.ErrNotExist) {
		return map[string]InstanceState{}, nil
	}
	if err != nil {
		return nil, err
	}
	states := map[string]InstanceState{}
	for _, entry := range entries {
		instance, ok := strings.CutSuffix(entry.Name(), ".json")
		if !ok || entry.IsDir() {
			continue
		}
		st, err := LoadState(stateDir, instance)
		if err != nil {
			return nil, err
		}
		states[instance] = st
	}
	return states, nil
}
