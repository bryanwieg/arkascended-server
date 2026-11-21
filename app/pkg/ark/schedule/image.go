package schedule

import (
	"context"
	"crypto/sha256"
	"debug/elf"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"

	"pkg/ark"
)

// ImageRepo is the local repository of scheduler images.
const ImageRepo = "ark-scheduler"

const schedulerDockerfile = `FROM scratch
COPY ark-cli /ark-cli
ENTRYPOINT ["/ark-cli"]
CMD ["run", "scheduler"]
`

// ImageRef returns "ark-scheduler:<first 12 hex chars of sha256(binary)>".
func ImageRef(binary string) (string, error) {
	data, err := os.ReadFile(binary)
	if err != nil {
		return "", fmt.Errorf("Error: read %s: %w", binary, err)
	}
	sum := sha256.Sum256(data)
	return ImageRepo + ":" + hex.EncodeToString(sum[:])[:12], nil
}

// CheckStatic fails unless binary is an ELF file without a PT_INTERP program header.
func CheckStatic(binary string) error {
	f, err := elf.Open(binary)
	if err != nil {
		return fmt.Errorf("Error: %s is not an ELF executable: %w", binary, err)
	}
	defer f.Close()
	for _, p := range f.Progs {
		if p.Type == elf.PT_INTERP {
			return fmt.Errorf("Error: %s is dynamically linked; the scheduler image needs a static ark-cli (build with CGO_ENABLED=0)", binary)
		}
	}
	return nil
}

// BuildImage builds ImageRef(binary) in buildDir (recreated) unless it exists; returns the ref and whether it built.
func BuildImage(ctx context.Context, engine ark.ContainerEngine, binary, buildDir string) (string, bool, error) {
	ref, err := ImageRef(binary)
	if err != nil {
		return "", false, err
	}
	exists, err := engine.ImageExists(ctx, ref)
	if err != nil {
		return "", false, fmt.Errorf("Error: check image %s: %w", ref, err)
	}
	if exists {
		return ref, false, nil
	}
	data, err := os.ReadFile(binary)
	if err != nil {
		return "", false, fmt.Errorf("Error: read %s: %w", binary, err)
	}
	dockerfile := filepath.Join(buildDir, "Dockerfile")
	if err := errors.Join(
		os.RemoveAll(buildDir),
		os.MkdirAll(buildDir, 0o755),
		os.WriteFile(filepath.Join(buildDir, "ark-cli"), data, 0o755), //nolint:gosec // G703: buildDir is the caller's own <compose dir>/.ark-manage path
		os.WriteFile(dockerfile, []byte(schedulerDockerfile), 0o644),
	); err != nil {
		return "", false, fmt.Errorf("Error: prepare scheduler build dir %s: %w", buildDir, err)
	}
	if err := engine.BuildImage(ctx, ark.BuildRequest{Dockerfile: dockerfile, Tag: ref, Context: buildDir}); err != nil {
		return "", false, fmt.Errorf("Error: scheduler image build failed: %w", err)
	}
	return ref, true, nil
}

// RemoveOldImages removes local ark-scheduler tags other than keep; returns removed refs (errors are joined).
func RemoveOldImages(ctx context.Context, engine ark.ContainerEngine, keep string) ([]string, error) {
	tags, err := engine.ImageTags(ctx, ImageRepo)
	if err != nil {
		return nil, fmt.Errorf("Error: list %s images: %w", ImageRepo, err)
	}
	refs := make([]string, 0, len(tags))
	for ref := range tags {
		if ref != keep {
			refs = append(refs, ref)
		}
	}
	sort.Strings(refs)
	var removed []string
	var errs []error
	for _, ref := range refs {
		if err := engine.RemoveImage(ctx, ref); err != nil {
			errs = append(errs, fmt.Errorf("remove %s: %w", ref, err))
			continue
		}
		removed = append(removed, ref)
	}
	return removed, errors.Join(errs...)
}
