package backup

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"pkg/ark"
	"pkg/ark/cmdrunner"
)

// Verify extracts a backup archive to a temp directory and checks SQLite
// integrity of all .sqlite files found.
func (s *Service) Verify(ctx context.Context, req VerifyRequest) (int, error) {
	tempDir, err := ark.TempDir("ark-backup-verify-")
	if err != nil {
		return 0, err
	}
	defer os.RemoveAll(tempDir)

	if err := s.ExtractArchive(ctx, req.ArchivePath, tempDir); err != nil {
		return 0, err
	}
	sqliteFiles, err := FindSQLiteFiles(tempDir)
	if err != nil {
		return 0, err
	}
	if len(sqliteFiles) == 0 {
		return 0, errors.New("Error: backup verification found no .sqlite files in the archive.")
	}

	failures := 0
	for _, path := range sqliteFiles {
		if err := s.CheckSQLiteIntegrity(ctx, path); err != nil {
			failures++
		}
	}
	return len(sqliteFiles), nil
}

// ExtractArchive extracts a backup archive to destDir using the service's
// command runner.
func (s *Service) ExtractArchive(ctx context.Context, archivePath, destDir string) error {
	if err := os.MkdirAll(destDir, 0o755); err != nil {
		return err
	}
	result, err := s.runner.Run(ctx, []string{"tar", "-xf", archivePath, "-C", destDir}, cmdrunner.RunOptions{Capture: true})
	if err != nil {
		return err
	}
	if result.ExitCode != 0 {
		detail := strings.TrimSpace(result.Stderr)
		if detail == "" {
			detail = strings.TrimSpace(result.Stdout)
		}
		if detail == "" {
			detail = fmt.Sprintf("tar extract failed with exit code %d", result.ExitCode)
		}
		return fmt.Errorf("Error: failed to extract backup archive: %s", detail)
	}
	return nil
}

// FindSQLiteFiles walks root and returns sorted paths of all .sqlite files.
func FindSQLiteFiles(root string) ([]string, error) {
	files := []string{}
	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}
		if strings.HasSuffix(strings.ToLower(d.Name()), ".sqlite") {
			files = append(files, path)
		}
		return nil
	})
	sort.Strings(files)
	return files, err
}

// CheckSQLiteIntegrity runs sqlite3 integrity_check on the given file.
func (s *Service) CheckSQLiteIntegrity(ctx context.Context, path string) error {
	if s.LookPath != nil {
		if _, err := s.LookPath("sqlite3"); err != nil {
			return errors.New("Error: sqlite3 is required for backup verification.")
		}
	}
	result, err := s.runner.Run(ctx, []string{"sqlite3", path, "PRAGMA integrity_check;"}, cmdrunner.RunOptions{Capture: true})
	if err != nil {
		return err
	}
	if result.ExitCode != 0 {
		detail := strings.TrimSpace(result.Stderr)
		if detail == "" {
			detail = strings.TrimSpace(result.Stdout)
		}
		if detail == "" {
			detail = fmt.Sprintf("sqlite3 exited with %d", result.ExitCode)
		}
		return fmt.Errorf("Error: sqlite integrity check failed for %s: %s", filepath.Base(path), detail)
	}
	if strings.TrimSpace(result.Stdout) != "ok" {
		return fmt.Errorf(
			"Error: sqlite integrity check failed for %s: integrity_check result: %s",
			filepath.Base(path),
			strings.TrimSpace(result.Stdout),
		)
	}
	return nil
}

// ApplyRetention applies the retention policy using the service's DiskUsage
// function and logger.
func (s *Service) ApplyRetention(destDir, instance string, policy RetentionPolicy) error {
	return ApplyRetention(destDir, instance, policy, s.DiskUsage, s.logger)
}

// ReadArchiveMapName lists the archive contents with tar and returns the map
// name found under SavedArks/<map>/, or "" if none is found.
func (s *Service) ReadArchiveMapName(ctx context.Context, archivePath string) (string, error) {
	result, err := s.runner.Run(ctx, []string{"tar", "-tf", archivePath}, cmdrunner.RunOptions{Capture: true})
	if err != nil {
		return "", fmt.Errorf("could not list backup archive contents: %v", err)
	}
	if result.ExitCode != 0 {
		return "", fmt.Errorf("could not list backup archive contents: tar exited with %d", result.ExitCode)
	}
	return ParseArchiveMapName(result.Stdout), nil
}

// ReadArchiveMetadata extracts backup-metadata.json from an archive and
// returns the parsed metadata. Returns an empty BackupMetadata (no error) when
// the metadata file is absent (backward-compatible with older archives).
func (s *Service) ReadArchiveMetadata(ctx context.Context, archivePath string) (BackupMetadata, error) {
	for _, memberName := range []string{"./backup-metadata.json", "backup-metadata.json"} {
		result, err := s.runner.Run(ctx, []string{"tar", "-O", "-xf", archivePath, memberName}, cmdrunner.RunOptions{Capture: true})
		if err != nil {
			return BackupMetadata{}, fmt.Errorf("could not read backup metadata: %v", err)
		}
		if result.ExitCode == 0 {
			m, parseErr := ParseArchiveMetadata(result.Stdout)
			if parseErr != nil {
				return BackupMetadata{}, fmt.Errorf("could not parse backup metadata: %v", parseErr)
			}
			return m, nil
		}
	}
	return BackupMetadata{}, nil
}

// ReadArchivePasswords returns the admin and server passwords from the archive's
// GameUserSettings.ini, or empty strings when the file is absent.
func (s *Service) ReadArchivePasswords(ctx context.Context, archivePath string) (admin, server string, err error) {
	const member = "Config/WindowsServer/GameUserSettings.ini"
	for _, memberName := range []string{"./" + member, member} {
		result, err := s.runner.Run(ctx, []string{"tar", "-O", "-xf", archivePath, memberName}, cmdrunner.RunOptions{Capture: true})
		if err != nil {
			return "", "", fmt.Errorf("could not read %s from backup archive: %w", member, err)
		}
		if result.ExitCode == 0 {
			admin, server = ParseServerPasswords(result.Stdout)
			return admin, server, nil
		}
	}
	return "", "", nil
}

// EnsureServiceContainer ensures a compose service container exists (creating it if needed).
func (s *Service) EnsureServiceContainer(ctx context.Context, composeFile, service string) error {
	services, err := s.compose.AllServices(ctx, composeFile)
	if err != nil {
		return err
	}
	if _, ok := services[service]; ok {
		return nil
	}
	return s.compose.Create(ctx, composeFile, []string{service})
}
