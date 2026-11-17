package core

import (
	"os"
	"path/filepath"
)

// Warnln is the function called by Die to print the error message before exiting.
// It is set to the CLI's verbose-aware warning printer during initialization.
// Default falls through to stderr so tests and non-CLI contexts do not panic.
var Warnln = func(message string) {
	_, _ = os.Stderr.WriteString(message + "\n")
}

// Die prints the message via Warnln and exits with code 1.
func Die(message string) {
	Warnln(message)
	os.Exit(1)
}

// YesNo returns "yes" or "no" for a boolean value.
func YesNo(v bool) string {
	if v {
		return "yes"
	}
	return "no"
}

// IsInteractiveTerminal reports whether stdin is an interactive terminal.
var IsInteractiveTerminal = func() bool {
	info, err := os.Stdin.Stat()
	if err != nil {
		return false
	}
	return (info.Mode() & os.ModeCharDevice) != 0
}

// BackupRootDir returns the default backup root directory.
func BackupRootDir() string {
	composeFile := os.Getenv("COMPOSE_FILE")
	if composeFile == "" {
		composeFile = "docker-compose.yaml"
	}
	return BackupRootDirForComposeFile(composeFile)
}

// BackupRootDirForComposeFile returns the backup root directory for a compose file.
func BackupRootDirForComposeFile(composeFile string) string {
	root := os.Getenv("ARK_BACKUP_DIR")
	if root != "" {
		return root
	}
	dir := filepath.Dir(composeFile)
	if abs, err := filepath.Abs(composeFile); err == nil {
		dir = filepath.Dir(abs)
	}
	return filepath.Join(dir, "backups")
}
