package ark

import (
	"os"
	"path/filepath"
)

// TempDir creates a temporary directory for ark-cli operations.
// Priority:
// 1. If ARK_TMP_DIR env var is set, use that path
// 2. Otherwise, use ./.ark-tmp/ relative to current working directory
func TempDir(prefix string) (string, error) {
	dir := os.Getenv("ARK_TMP_DIR")
	if dir == "" {
		cwd, err := os.Getwd()
		if err != nil {
			return "", err
		}
		dir = filepath.Join(cwd, ".ark-tmp")
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", err
	}
	return os.MkdirTemp(dir, prefix)
}
