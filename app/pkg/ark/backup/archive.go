package backup

import (
	"archive/tar"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"
	"unicode/utf16"
)

// ArchiveEntry holds metadata for a backup archive file.
type ArchiveEntry struct {
	Path    string
	Info    os.FileInfo
	sortKey archiveSortValue
}

// TimestampPattern matches the -YYYYMMDD-HHMMSS suffix every archive name ends
// with (regular, --name, pre-update and pre-restore).
var TimestampPattern = regexp.MustCompile(`-(\d{8}-\d{6})$`)

// ErrArchiveNotFound builds an error for a missing backup archive.
func ErrArchiveNotFound(backupsDir, backupName string) error {
	return fmt.Errorf(
		"Error: backup archive not found for instance %s: %s",
		filepath.Base(backupsDir),
		backupName,
	)
}

// ResolveArchive finds a backup archive by name or "latest".
func ResolveArchive(backupsDir, backupName string) (string, error) {
	if backupName == "latest" {
		candidates, err := ListArchives(backupsDir)
		if err != nil {
			return "", err
		}
		if len(candidates) == 0 {
			return "", ErrArchiveNotFound(backupsDir, backupName)
		}
		return candidates[0], nil
	}

	if HasArchiveSuffix(backupName) {
		candidate := filepath.Join(backupsDir, backupName)
		if _, err := os.Stat(candidate); err == nil {
			return candidate, nil
		}
	} else {
		for _, suffix := range ArchiveSuffixes {
			candidate := filepath.Join(backupsDir, backupName+suffix)
			if _, err := os.Stat(candidate); err == nil {
				return candidate, nil
			}
		}
	}
	return "", ErrArchiveNotFound(backupsDir, backupName)
}

// ListArchives lists backup archives in a directory, sorted newest first.
func ListArchives(backupsDir string) ([]string, error) {
	entries, err := os.ReadDir(backupsDir)
	if err != nil {
		return nil, err
	}
	archives := []ArchiveEntry{}
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		name := entry.Name()
		if HasArchiveSuffix(name) {
			path := filepath.Join(backupsDir, name)
			info, statErr := entry.Info()
			if statErr != nil {
				return nil, statErr
			}
			archives = append(archives, ArchiveEntry{
				Path:    path,
				Info:    info,
				sortKey: archiveSortKey(path, info),
			})
		}
	}
	sort.Slice(archives, func(i, j int) bool {
		return archives[i].sortKey.newerThan(archives[j].sortKey)
	})
	files := make([]string, 0, len(archives))
	for _, archive := range archives {
		files = append(files, archive.Path)
	}
	return files, nil
}

// CompareArchiveSort compares two backup archives for sort ordering (newest first).
func CompareArchiveSort(left string, leftInfo os.FileInfo, right string, rightInfo os.FileInfo) bool {
	return archiveSortKey(left, leftInfo).newerThan(archiveSortKey(right, rightInfo))
}

type archiveSortValue struct {
	priority int
	value    int64
	mtime    int64 // tiebreak: archives written in the same second share a name timestamp
}

func (k archiveSortValue) newerThan(o archiveSortValue) bool {
	if k.priority != o.priority {
		return k.priority > o.priority
	}
	if k.value != o.value {
		return k.value > o.value
	}
	return k.mtime > o.mtime
}

func archiveSortKey(path string, info os.FileInfo) archiveSortValue {
	var mtime int64
	if info != nil {
		mtime = info.ModTime().UnixNano()
	}
	stem := TrimArchiveSuffix(filepath.Base(path))
	match := TimestampPattern.FindStringSubmatch(stem)
	if len(match) > 1 {
		flattened := strings.ReplaceAll(match[1], "-", "")
		if parsed, err := strconv.ParseInt(flattened, 10, 64); err == nil {
			return archiveSortValue{priority: 1, value: parsed, mtime: mtime}
		}
	}
	return archiveSortValue{priority: 0, value: mtime / int64(time.Second), mtime: mtime}
}

// IsArchiveForInstance checks if a filename matches the backup naming pattern
// for the given instance.
func IsArchiveForInstance(instance, name string) bool {
	if !strings.HasPrefix(name, instance+"-") {
		return false
	}
	return HasArchiveSuffix(name)
}

// ParseArchiveMapName scans tar -tf output for a SavedArks/<mapname>/ entry
// and returns the map name, or "" if not found.
func ParseArchiveMapName(listing string) string {
	for _, line := range strings.Split(listing, "\n") {
		line = strings.TrimPrefix(strings.TrimSpace(line), "./")
		parts := strings.SplitN(line, "/", 3)
		if len(parts) >= 2 && parts[0] == "SavedArks" && parts[1] != "" {
			return parts[1]
		}
	}
	return ""
}

// EnsureTarWithinBase validates that no tar entry escapes the target directory.
func EnsureTarWithinBase(members []*tar.Header, base string) error {
	baseAbs, err := filepath.Abs(base)
	if err != nil {
		return err
	}
	for _, member := range members {
		target := filepath.Clean(filepath.Join(baseAbs, member.Name))
		if target == baseAbs {
			continue
		}
		if !strings.HasPrefix(target, baseAbs+string(os.PathSeparator)) {
			return fmt.Errorf("Error: tar entry escapes target directory: %s", member.Name)
		}
	}
	return nil
}

// EnsureNoOverwrite checks that no tar entry would overwrite an existing file.
func EnsureNoOverwrite(members []*tar.Header, base string) error {
	for _, member := range members {
		target := filepath.Join(base, member.Name)
		if _, err := os.Stat(target); err == nil {
			return fmt.Errorf("Error: existing path blocks restore (use --force): %s", target)
		}
	}
	return nil
}

// DirectoryForInstance returns the backup directory for a given instance,
// or an error if it doesn't exist.
func DirectoryForInstance(backupRoot, instance string) (string, error) {
	backupsDir := filepath.Join(backupRoot, instance)
	if _, err := os.Stat(backupsDir); err != nil {
		return "", fmt.Errorf("Error: backup directory not found for instance: %s", instance)
	}
	return backupsDir, nil
}

// HasAnyArchiveForInstance checks if at least one backup archive exists for an instance.
func HasAnyArchiveForInstance(backupRoot, instance string) bool {
	backupsDir := filepath.Join(backupRoot, instance)
	entries, err := os.ReadDir(backupsDir)
	if err != nil {
		return false
	}
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		if IsArchiveForInstance(instance, entry.Name()) {
			return true
		}
	}
	return false
}

// BackupMetadata holds the metadata embedded in a backup archive.
type BackupMetadata struct {
	ServerVersion   string            `json:"server_version"`
	BuildID         string            `json:"build_id"`
	InstanceName    string            `json:"instance_name"`
	MapName         string            `json:"map_name"`
	BackupTimestamp string            `json:"backup_timestamp"`
	Environment     map[string]string `json:"environment"`
}

// ParseArchiveMetadata parses raw JSON into a BackupMetadata struct.
func ParseArchiveMetadata(raw string) (BackupMetadata, error) {
	var m BackupMetadata
	if err := json.Unmarshal([]byte(raw), &m); err != nil {
		return BackupMetadata{}, err
	}
	return m, nil
}

// ParseServerPasswords returns ServerAdminPassword and ServerPassword from GameUserSettings.ini
// content. The game may rewrite the file as UTF-16, so a UTF-16 byte-order mark is decoded first.
func ParseServerPasswords(ini string) (admin, server string) {
	for _, line := range strings.Split(decodeINIText(ini), "\n") {
		key, value, ok := strings.Cut(strings.TrimSpace(line), "=")
		switch {
		case !ok:
		case key == "ServerAdminPassword":
			admin = value
		case key == "ServerPassword":
			server = value
		}
	}
	return admin, server
}

func decodeINIText(text string) string {
	bigEndian := strings.HasPrefix(text, "\xFE\xFF")
	if !bigEndian && !strings.HasPrefix(text, "\xFF\xFE") {
		return strings.TrimPrefix(text, "\uFEFF")
	}
	b := []byte(text)
	units := make([]uint16, 0, len(b)/2)
	for i := 2; i+1 < len(b); i += 2 {
		if bigEndian {
			units = append(units, uint16(b[i])<<8|uint16(b[i+1]))
		} else {
			units = append(units, uint16(b[i+1])<<8|uint16(b[i]))
		}
	}
	return string(utf16.Decode(units))
}
