package backup

import (
	"ark-cli/internal/core"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

type SourcePath struct {
	Abs string
	Rel string
}

// absPath resolves a path to absolute, falling back to the relative path if
// os.Getwd fails (an extremely rare scenario where cwd is deleted mid-process).
func absPath(path string) string {
	if abs, err := filepath.Abs(path); err == nil {
		return abs
	}
	return path
}

func ResolveSourcePaths(d *core.AppDeps, paths []string) ([]SourcePath, error) {
	base := core.RepoRoot()
	resolved := make([]SourcePath, 0, len(paths))
	for _, raw := range paths {
		if mount, ok := d.Engine.VolumeMountpoint(d.Context(), raw); ok {
			absSource := absPath(mount)
			if _, err := os.Stat(absSource); err != nil {
				return nil, fmt.Errorf("Error: backup source not found: %s", raw)
			}
			resolved = append(resolved, SourcePath{Abs: absSource, Rel: raw})
			continue
		}

		absSource := absPath(filepath.Join(base, raw))
		if filepath.IsAbs(raw) {
			absSource = absPath(raw)
		}
		baseAbs := absPath(base)
		prefix := baseAbs + string(os.PathSeparator)
		if absSource != baseAbs && !strings.HasPrefix(absSource, prefix) {
			return nil, fmt.Errorf("Error: backup source must be inside repo or a container volume: %s", raw)
		}
		if _, err := os.Stat(absSource); err != nil {
			return nil, fmt.Errorf("Error: backup source not found: %s", raw)
		}
		// Both baseAbs and absSource are guaranteed absolute by absPath(),
		// so filepath.Rel cannot fail here.
		relSource, err := filepath.Rel(baseAbs, absSource)
		if err != nil {
			panic("unreachable: filepath.Rel failed on absolute paths: " + err.Error())
		}
		resolved = append(resolved, SourcePath{Abs: absSource, Rel: relSource})
	}
	return resolved, nil
}
