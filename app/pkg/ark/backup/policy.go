package backup

import (
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

const (
	MinFreeBackupBytes   int64 = 5 * 1024 * 1024 * 1024
	MinSingleBackupBytes int64 = 1 * 1024 * 1024 * 1024
	MinKeepNormalBackups int   = 2
	MinKeepLowSpace      int   = 1
)

// ApplyRetention applies the retention policy to backups in destDir for the
// given instance. diskUsage is called to determine free space. logger may be
// nil for silent operation.
func ApplyRetention(destDir, instance string, policy RetentionPolicy, diskUsage DiskUsageFn, logger *slog.Logger) error {
	files, err := ListRetentionCandidates(destDir, instance)
	if err != nil {
		return err
	}
	if len(files) == 0 {
		return nil
	}
	if err := SortByNewest(files); err != nil {
		return err
	}

	freeBytes, _, err := diskUsage(destDir)
	if err != nil {
		return err
	}
	minKeep := retentionMinKeep(destDir, freeBytes, len(files), logger)
	keepByCount := retentionKeepByCount(policy, len(files), minKeep)

	state, err := newRetentionState(files, minKeep, keepByCount)
	if err != nil {
		return err
	}

	pruneByAge(&state, policy, logger)
	pruneByCount(&state, policy, logger)
	pruneBySize(&state, policy, logger)
	pruneForFreeSpace(&state, freeBytes, logger)

	return removeRetainedBackups(files, state.removeSet)
}

// DryRunRetention calculates what would be pruned without removing anything.
// Returns (wouldRemove, wouldKeep) lists.
func DryRunRetention(destDir, instance string, policy RetentionPolicy, diskUsage DiskUsageFn, logger *slog.Logger) ([]PruneResult, error) {
	files, err := ListRetentionCandidates(destDir, instance)
	if err != nil {
		return nil, err
	}
	if len(files) == 0 {
		return nil, nil
	}
	if err := SortByNewest(files); err != nil {
		return nil, err
	}

	freeBytes, _, err := diskUsage(destDir)
	if err != nil {
		return nil, err
	}
	minKeep := retentionMinKeep(destDir, freeBytes, len(files), logger)
	keepByCount := retentionKeepByCount(policy, len(files), minKeep)

	state, err := newRetentionState(files, minKeep, keepByCount)
	if err != nil {
		return nil, err
	}

	pruneByAge(&state, policy, logger)
	pruneByCount(&state, policy, logger)
	pruneBySize(&state, policy, logger)
	pruneForFreeSpace(&state, freeBytes, logger)

	results := make([]PruneResult, 0, len(files))
	for _, path := range files {
		results = append(results, PruneResult{
			Path:   path,
			Remove: state.removeSet[path],
			Reason: state.reasonMap[path],
		})
	}
	return results, nil
}

// PruneResult describes the fate of a single backup file.
type PruneResult struct {
	Path   string
	Remove bool
	Reason string
}

type retentionState struct {
	files       []string
	fileSizes   map[string]int64
	fileTimes   map[string]time.Time
	removeSet   map[string]bool
	reasonMap   map[string]string
	keptCount   int
	totalSize   int64
	minKeep     int
	keepByCount int
}

// ListRetentionCandidates returns backup archive paths for the given instance
// in the specified directory, suitable for retention processing.
func ListRetentionCandidates(destDir, instance string) ([]string, error) {
	entries, err := os.ReadDir(destDir)
	if err != nil {
		return nil, err
	}
	files := []string{}
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		name := entry.Name()
		if !IsArchiveForInstance(instance, name) {
			continue
		}
		files = append(files, filepath.Join(destDir, name))
	}
	return files, nil
}

func retentionMinKeep(destDir string, freeBytes int64, fileCount int, logger *slog.Logger) int {
	minKeep := MinKeepNormalBackups
	if freeBytes < MinFreeBackupBytes {
		minKeep = MinKeepLowSpace
		if logger != nil {
			logger.Warn("low free disk space for backups; retention will keep at least 1 backup",
				"path", destDir, "free_bytes", freeBytes)
		}
	}
	if minKeep > fileCount {
		return fileCount
	}
	return minKeep
}

func retentionKeepByCount(policy RetentionPolicy, fileCount, minKeep int) int {
	keepByCount := fileCount
	if policy.RetainCount > 0 && policy.RetainCount < keepByCount {
		keepByCount = policy.RetainCount
	}
	if keepByCount < minKeep {
		return minKeep
	}
	return keepByCount
}

func newRetentionState(files []string, minKeep, keepByCount int) (retentionState, error) {
	totalSize := int64(0)
	fileSizes := map[string]int64{}
	fileTimes := map[string]time.Time{}
	for _, path := range files {
		info, err := os.Stat(path)
		if err != nil {
			return retentionState{}, err
		}
		fileSizes[path] = info.Size()
		fileTimes[path] = info.ModTime()
		totalSize += info.Size()
	}
	return retentionState{
		files:       files,
		fileSizes:   fileSizes,
		fileTimes:   fileTimes,
		removeSet:   map[string]bool{},
		reasonMap:   map[string]string{},
		keptCount:   len(files),
		totalSize:   totalSize,
		minKeep:     minKeep,
		keepByCount: keepByCount,
	}, nil
}

func (s *retentionState) markForRemoval(path, reason string, logger *slog.Logger) {
	if s.removeSet[path] {
		return
	}
	s.removeSet[path] = true
	s.reasonMap[path] = reason
	s.keptCount--
	s.totalSize -= s.fileSizes[path]
	if logger != nil {
		logger.Info("retention removed backup", "path", path, "reason", reason)
	}
}

func pruneByAge(state *retentionState, policy RetentionPolicy, logger *slog.Logger) {
	if policy.RetainDays <= 0 || state.keptCount <= state.minKeep {
		return
	}
	threshold := time.Now().Add(-time.Duration(policy.RetainDays) * 24 * time.Hour)
	for idx := len(state.files) - 1; idx >= 0 && state.keptCount > state.minKeep; idx-- {
		path := state.files[idx]
		if state.removeSet[path] {
			continue
		}
		if state.fileTimes[path].Before(threshold) {
			state.markForRemoval(path, fmt.Sprintf("older than %d day retention", policy.RetainDays), logger)
		}
	}
}

func pruneByCount(state *retentionState, policy RetentionPolicy, logger *slog.Logger) {
	for idx := len(state.files) - 1; idx >= 0 && state.keptCount > state.keepByCount; idx-- {
		path := state.files[idx]
		if state.removeSet[path] {
			continue
		}
		if state.keptCount <= state.minKeep {
			break
		}
		state.markForRemoval(path, fmt.Sprintf("exceeds retain-count=%d", policy.RetainCount), logger)
	}
}

func pruneBySize(state *retentionState, policy RetentionPolicy, logger *slog.Logger) {
	if policy.RetainSizeBytes <= 0 || state.totalSize <= policy.RetainSizeBytes {
		return
	}
	for idx := len(state.files) - 1; idx >= 0 && state.totalSize > policy.RetainSizeBytes; idx-- {
		path := state.files[idx]
		if state.removeSet[path] {
			continue
		}
		if state.keptCount <= state.minKeep {
			if logger != nil {
				logger.Warn("retain-size-gb hard limit exceeded but minimum backup floor prevents additional pruning",
					"total_size", state.totalSize, "limit", policy.RetainSizeBytes, "min_keep", state.minKeep)
			}
			break
		}
		state.markForRemoval(path, fmt.Sprintf("exceeds retain-size-bytes=%d", policy.RetainSizeBytes), logger)
	}
}

func pruneForFreeSpace(state *retentionState, freeBytes int64, logger *slog.Logger) {
	projectedFree := freeBytes + sumRemovedBytes(state.removeSet, state.fileSizes)
	if projectedFree >= MinFreeBackupBytes {
		return
	}
	for idx := len(state.files) - 1; idx >= 0 && projectedFree < MinFreeBackupBytes; idx-- {
		path := state.files[idx]
		if state.removeSet[path] {
			continue
		}
		if state.keptCount <= state.minKeep {
			if logger != nil {
				logger.Warn("free-space reserve target not met after retention floor",
					"projected_free", projectedFree, "target", MinFreeBackupBytes, "min_keep", state.minKeep)
			}
			break
		}
		state.markForRemoval(path, "free-space reserve enforcement", logger)
		projectedFree += state.fileSizes[path]
	}
}

func removeRetainedBackups(files []string, removeSet map[string]bool) error {
	for _, path := range files {
		if !removeSet[path] {
			continue
		}
		if err := os.Remove(path); err != nil {
			return err
		}
	}
	return nil
}

func sumRemovedBytes(removeSet map[string]bool, fileSizes map[string]int64) int64 {
	total := int64(0)
	for path, remove := range removeSet {
		if !remove {
			continue
		}
		total += fileSizes[path]
	}
	return total
}

// SortByNewest sorts backup file paths by timestamp (newest first), falling
// back to modification time.
func SortByNewest(files []string) error {
	fileInfoByPath := make(map[string]os.FileInfo, len(files))
	for _, path := range files {
		info, err := os.Stat(path)
		if err != nil {
			return err
		}
		fileInfoByPath[path] = info
	}
	sort.Slice(files, func(i, j int) bool {
		return CompareArchiveSort(
			files[i],
			fileInfoByPath[files[i]],
			files[j],
			fileInfoByPath[files[j]],
		)
	})
	return nil
}

// EnsureHeadroomForStart checks that there's enough disk space for backups
// before starting services.
func EnsureHeadroomForStart(backupRoot string, services []string, diskUsage DiskUsageFn, logger *slog.Logger) error {
	statPath := backupRoot
	for {
		if _, err := os.Stat(statPath); err == nil {
			break
		}
		next := filepath.Dir(statPath)
		if next == statPath {
			return fmt.Errorf("Error: unable to find existing path for disk-space check: %s", backupRoot)
		}
		statPath = next
	}
	freeBytes, _, err := diskUsage(statPath)
	if err != nil {
		return err
	}
	if freeBytes >= MinFreeBackupBytes {
		return nil
	}

	if logger != nil {
		logger.Warn("low free disk space for backups",
			"path", backupRoot, "free_bytes", freeBytes, "target_reserve", MinFreeBackupBytes)
	}

	if freeBytes <= 0 {
		return fmt.Errorf("Error: insufficient disk space for server to run (no free space available for backups)")
	}
	if freeBytes >= MinSingleBackupBytes {
		return nil
	}

	missingBackup := []string{}
	for _, service := range services {
		if !HasAnyArchiveForInstance(backupRoot, service) {
			missingBackup = append(missingBackup, service)
		}
	}
	if len(missingBackup) > 0 {
		return fmt.Errorf(
			"Error: insufficient disk space for server to run; no room to keep backups for service(s): %s",
			strings.Join(missingBackup, ", "),
		)
	}
	return nil
}
