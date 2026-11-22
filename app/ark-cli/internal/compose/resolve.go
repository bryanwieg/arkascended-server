package compose

import (
	"ark-cli/internal/core"
	"errors"
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

// WriteComposeText replaces composeFile with text: the write path for adding or removing a service, which
// SaveProject refuses. The text goes to a copy next to the file (so .env and the project name resolve the
// same), which must load and pass check (nil: load only) before it is renamed over the file; on any
// failure the file is left unchanged.
func WriteComposeText(d *core.AppDeps, composeFile, text string, check func(*ark.ComposeProject) error) error {
	// Write through a symlinked compose file; renaming over the link would replace it with a copy.
	if target, err := filepath.EvalSymlinks(composeFile); err == nil {
		composeFile = target
	}
	mode := os.FileMode(0o644)
	if info, err := os.Stat(composeFile); err == nil {
		mode = info.Mode().Perm()
	} else if !errors.Is(err, os.ErrNotExist) {
		return err
	}
	tmp := filepath.Join(filepath.Dir(composeFile), "."+filepath.Base(composeFile)+".ark-save.tmp")
	err := os.WriteFile(tmp, []byte(text), mode)
	if err == nil {
		var project *ark.ComposeProject
		if project, err = d.ComposeService.LoadProject(d.Context(), tmp); err == nil && check != nil {
			err = check(project)
		}
	}
	if err == nil {
		err = os.Rename(tmp, composeFile)
	}
	if err != nil {
		_ = os.Remove(tmp)
		return err
	}
	return nil
}
