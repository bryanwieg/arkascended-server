package compose

import (
	"ark-cli/internal/core"
	"fmt"
	"os"
	"path/filepath"

	"pkg/ark"
)

func RequireComposeFile(composeFile string) error {
	if _, err := os.Stat(composeFile); err != nil {
		return fmt.Errorf("Error: compose file not found: %s", composeFile)
	}
	return nil
}

func EnvOrDefault(key, fallback string) string {
	value := os.Getenv(key)
	if value == "" {
		return fallback
	}
	return value
}

func ResolveComposeFile(flagValue string) string {
	if flagValue != "" {
		return flagValue
	}
	return EnvOrDefault("COMPOSE_FILE", "docker-compose.yaml")
}

func ResolveRequiredComposeConfig(d *core.AppDeps, composeFileFlag string) (string, *ark.ComposeProject, error) {
	composeFile := ResolveComposeFile(composeFileFlag)
	project, err := d.RuntimeHelper.LoadComposeProject(composeFile)
	if err != nil {
		return "", nil, err
	}
	return composeFile, project, nil
}

// ComposeDirIsCwd reports whether the compose file's directory is the working directory (compared with
// os.SameFile, so symlinked paths such as /home -> /var/home match). The schedule store is read from the
// working directory on the host and from the compose dir in the scheduler, so the two must agree.
func ComposeDirIsCwd(composeFile string) (composeDir, cwd string, same bool, err error) {
	abs, err := filepath.Abs(composeFile)
	if err != nil {
		return "", "", false, err
	}
	composeDir = filepath.Dir(abs)
	if cwd, err = os.Getwd(); err != nil {
		return composeDir, "", false, err
	}
	cwdInfo, cwdErr := os.Stat(cwd)
	dirInfo, dirErr := os.Stat(composeDir)
	return composeDir, cwd, cwdErr == nil && dirErr == nil && os.SameFile(cwdInfo, dirInfo), nil
}
