package schedule

import (
	"context"
	"fmt"
	"slices"
	"strings"

	"pkg/ark"
)

// LoadScheduleFile reads the schedules from the compose file's x-ark-schedules block; no block means
// no schedules. Unknown keys of an entry are ignored.
func (s *Service) LoadScheduleFile(ctx context.Context, compose ark.ComposeService, composeFile string) (ScheduleFile, error) {
	project, err := compose.LoadProject(ctx, composeFile)
	if err != nil {
		return ScheduleFile{}, err
	}
	value, ok := project.Project.Extensions[ark.SchedulesExtensionKey]
	if !ok {
		return ScheduleFile{Schedules: []BackupSchedule{}}, nil
	}
	bad := func(reason string) (ScheduleFile, error) {
		return ScheduleFile{}, fmt.Errorf("Error: %s in %s: %s", ark.SchedulesExtensionKey, composeFile, reason)
	}
	items, ok := value.([]any)
	if !ok {
		return bad("must be a list of schedule entries")
	}
	schedules := make([]BackupSchedule, 0, len(items))
	for i, item := range items {
		fields, ok := item.(map[string]any)
		if !ok {
			return bad(fmt.Sprintf("entry %d must be a mapping", i+1))
		}
		sched, err := entryFromFields(fields)
		if err != nil {
			return bad(fmt.Sprintf("entry %d: %v", i+1, err))
		}
		schedules = append(schedules, sched)
	}
	return ScheduleFile{Schedules: schedules}, nil
}

// SaveScheduleFile writes the schedules, sorted by name, to the compose file's x-ark-schedules block;
// an empty list removes the block.
func (s *Service) SaveScheduleFile(ctx context.Context, compose ark.ComposeService, composeFile string, file ScheduleFile) error {
	project, err := compose.LoadProject(ctx, composeFile)
	if err != nil {
		return err
	}
	sorted := slices.SortedFunc(slices.Values(file.Schedules), func(a, b BackupSchedule) int {
		return strings.Compare(a.Name, b.Name)
	})
	if project.Project.Extensions == nil {
		project.Project.Extensions = map[string]any{}
	}
	if len(sorted) == 0 {
		delete(project.Project.Extensions, ark.SchedulesExtensionKey)
	} else {
		items := make([]any, 0, len(sorted))
		for _, sched := range sorted {
			items = append(items, entryFields(sched))
		}
		project.Project.Extensions[ark.SchedulesExtensionKey] = items
	}
	return compose.SaveProject(ctx, composeFile, project)
}

// entryFields is the compose form of an entry: string, bool and int values, zero optional fields left out.
func entryFields(s BackupSchedule) map[string]any {
	fields := map[string]any{"name": s.Name, "type": s.Type, "instance": s.Instance, "enabled": s.Enabled, "cron": s.Cron}
	if s.Command != "" {
		fields["command"] = s.Command
	}
	if s.Format != "" {
		fields["format"] = s.Format
	}
	for key, n := range map[string]int{"retain_count": s.RetainCount, "retain_days": s.RetainDays, "retain_size_gb": s.RetainSizeGB} {
		if n != 0 {
			fields[key] = n
		}
	}
	return fields
}

// entryFromFields decodes entryFields' form; a missing key is the zero value, a value of another type an error.
func entryFromFields(fields map[string]any) (BackupSchedule, error) {
	var s BackupSchedule
	for key, target := range map[string]any{
		"name": &s.Name, "type": &s.Type, "instance": &s.Instance, "enabled": &s.Enabled, "cron": &s.Cron,
		"command": &s.Command, "format": &s.Format,
		"retain_count": &s.RetainCount, "retain_days": &s.RetainDays, "retain_size_gb": &s.RetainSizeGB,
	} {
		value, present := fields[key]
		if !present {
			continue
		}
		ok := false
		switch t := target.(type) {
		case *string:
			*t, ok = value.(string)
		case *bool:
			*t, ok = value.(bool)
		case *int:
			*t, ok = value.(int)
		}
		if !ok {
			return BackupSchedule{}, fmt.Errorf("%s has the wrong type (%T)", key, value)
		}
	}
	return s, nil
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

// LoadScheduleByName loads the schedules of the compose file and looks up a single entry.
func (s *Service) LoadScheduleByName(ctx context.Context, compose ark.ComposeService, composeFile, name string) (ScheduleFile, BackupSchedule, int, error) {
	file, err := s.LoadScheduleFile(ctx, compose, composeFile)
	if err != nil {
		return ScheduleFile{}, BackupSchedule{}, -1, err
	}
	sched, index, ok := FindScheduleByName(file, name)
	if !ok {
		return ScheduleFile{}, BackupSchedule{}, -1, ErrNotFound(name)
	}
	return file, sched, index, nil
}
