package schedule

import (
	"os"
	"path/filepath"
	"sort"

	"github.com/goccy/go-yaml"
)

// LoadScheduleFile reads the schedule config from disk. Returns an empty
// file with Version=1 if the config does not exist yet.
func (s *Service) LoadScheduleFile() (ScheduleFile, error) {
	data, err := os.ReadFile(s.ConfigPath)
	if err != nil {
		if os.IsNotExist(err) {
			return ScheduleFile{Version: 1, Schedules: []BackupSchedule{}}, nil
		}
		return ScheduleFile{}, err
	}
	var file ScheduleFile
	if err := yaml.Unmarshal(data, &file); err != nil {
		return ScheduleFile{}, err
	}
	if file.Version == 0 {
		file.Version = 1
	}
	if file.Schedules == nil {
		file.Schedules = []BackupSchedule{}
	}
	return file, nil
}

// SaveScheduleFile writes the schedule config to disk, sorted by name.
func (s *Service) SaveScheduleFile(file ScheduleFile) error {
	if file.Version == 0 {
		file.Version = 1
	}
	if file.Schedules == nil {
		file.Schedules = []BackupSchedule{}
	}
	sort.Slice(file.Schedules, func(i, j int) bool { return file.Schedules[i].Name < file.Schedules[j].Name })
	data, err := yaml.Marshal(file)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(s.ConfigPath), 0o755); err != nil {
		return err
	}
	return os.WriteFile(s.ConfigPath, data, 0o644)
}

// FindScheduleByName looks up a schedule entry by name.
func FindScheduleByName(file ScheduleFile, name string) (BackupSchedule, int, bool) {
	for i, sched := range file.Schedules {
		if sched.Name == name {
			return sched, i, true
		}
	}
	return BackupSchedule{}, -1, false
}

// LoadScheduleByName loads the schedule file and looks up a single entry.
func (s *Service) LoadScheduleByName(name string) (ScheduleFile, BackupSchedule, int, error) {
	file, err := s.LoadScheduleFile()
	if err != nil {
		return ScheduleFile{}, BackupSchedule{}, -1, err
	}
	sched, index, ok := FindScheduleByName(file, name)
	if !ok {
		return ScheduleFile{}, BackupSchedule{}, -1, ErrNotFound(name)
	}
	return file, sched, index, nil
}
