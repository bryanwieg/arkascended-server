package compose

import (
	"ark-cli/internal/core"
	"fmt"
	"os"

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
