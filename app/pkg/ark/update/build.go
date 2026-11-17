package update

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	arkcontainer "ark-container"
	"pkg/ark"
	"pkg/ark/backup"
	"pkg/ark/composeutil"

	"github.com/compose-spec/compose-go/v2/types"
)

// FilesImageRepo is the local server files image, built from the SteamCMD
// download and removed once the runtime image is built.
const FilesImageRepo = "ark-server-files"

// StateDir returns <compose file's directory>/.ark-manage as an absolute path.
func StateDir(composeFile string) string {
	dir, err := filepath.Abs(filepath.Dir(composeFile))
	if err != nil {
		dir = filepath.Dir(composeFile)
	}
	return filepath.Join(dir, ".ark-manage")
}

// BuildResult describes the image BuildLocalImage produced or found.
type BuildResult struct {
	Image   string // ark-server:<build ID>
	BuildID string
	Version string
	Built   bool // false when the image already existed
}

// BuildLocalImage builds ark-server:<Steam's current ARK build ID> (also tagged
// :<version> and :latest) from the recipe embedded in this binary, downloading
// the server with SteamCMD into stateDir/build/work. An existing image for that
// build is returned without building.
func (s *Service) BuildLocalImage(ctx context.Context, stateDir string) (BuildResult, error) {
	buildID, err := s.FetchSteamLatestBuildID()
	if err != nil {
		return BuildResult{}, fmt.Errorf("Error: cannot read ARK's current build from Steam: %w", err)
	}
	image := DefaultLocalImageName + ":" + buildID
	if result, ok, err := s.existingBuild(ctx, image, buildID); err != nil || ok {
		return result, err
	}
	if err := os.MkdirAll(stateDir, 0o755); err != nil {
		return BuildResult{}, err
	}
	var result BuildResult
	err = backup.WithInstanceLock(filepath.Join(stateDir, "locks"), "image-build", 6*time.Hour, func() error {
		existing, ok, err := s.existingBuild(ctx, image, buildID) // another process may have built it meanwhile
		if err != nil || ok {
			result = existing
			return err
		}
		built, err := s.buildImage(ctx, stateDir, buildID)
		result = built
		return err
	})
	return result, err
}

func (s *Service) existingBuild(ctx context.Context, image, buildID string) (BuildResult, bool, error) {
	exists, err := s.engine.ImageExists(ctx, image)
	if err != nil || !exists {
		return BuildResult{}, false, err
	}
	info, err := s.InspectImageVersionInfo(ctx, image)
	if err != nil {
		return BuildResult{}, false, err
	}
	return BuildResult{Image: image, BuildID: buildID, Version: info.Version}, true, nil
}

func (s *Service) buildImage(ctx context.Context, stateDir, buildID string) (BuildResult, error) {
	if err := s.checkBuildDisk(stateDir); err != nil {
		return BuildResult{}, err
	}
	contextDir := filepath.Join(stateDir, "build", "context")
	if err := arkcontainer.Extract(contextDir); err != nil {
		return BuildResult{}, fmt.Errorf("Error: could not write the image build files to %s: %w", contextDir, err)
	}
	baseImage, err := s.ensureBaseImage(ctx, contextDir)
	if err != nil {
		return BuildResult{}, err
	}

	filesImage := FilesImageRepo + ":" + buildID
	s.logger.Info("downloading the ARK server with SteamCMD", "build_id", buildID)
	runtime := s.ContainerRuntime
	if runtime == "" {
		runtime = "docker"
	}
	cmd := []string{"env",
		"ARK_STEAM_BUILD_ID=" + buildID,
		"CONTAINER_RUNTIME=" + runtime,
		"WORK_DIR=" + filepath.Join(stateDir, "build", "work"),
		"bash", filepath.Join(contextDir, "files", "build.sh"), FilesImageRepo}
	run, err := s.RunCommand(cmd, ark.CommandOptions{})
	if err == nil && run.ExitCode != 0 {
		err = fmt.Errorf("exit code %d", run.ExitCode)
	}
	if err != nil {
		return BuildResult{}, fmt.Errorf("Error: server files build failed: %w", err)
	}

	baseTag := DefaultLocalImageName + ":" + LocalBaseTagSuffix
	s.logger.Info("building base image", "tag", baseTag, "files_image", filesImage)
	if err := s.engine.BuildImage(ctx, ark.BuildRequest{
		Dockerfile: filepath.Join(contextDir, LocalDockerfilePath),
		Tag:        baseTag,
		Context:    contextDir,
		NoCache:    envOrDefault("ARK_BUILD_NO_CACHE", "") != "",
		BuildArgs:  map[string]string{"BASE_IMAGE": baseImage, "ARK_FILES_IMAGE": filesImage},
	}); err != nil {
		return BuildResult{}, fmt.Errorf("Error: base image build failed: %w", err)
	}
	version, err := s.engine.ReadFileFromImage(ctx, baseTag, ContainerVersionPath)
	if err != nil {
		return BuildResult{}, fmt.Errorf("Error: failed to read version from base image: %w", err)
	}
	imageBuildID, err := s.engine.ReadFileFromImage(ctx, baseTag, ContainerBuildIDPath)
	if err != nil {
		return BuildResult{}, fmt.Errorf("Error: failed to read build id from base image: %w", err)
	}
	if imageBuildID != buildID {
		return BuildResult{}, fmt.Errorf("Error: built ARK build %s, but Steam reported %s; run the update again", imageBuildID, buildID)
	}

	image := DefaultLocalImageName + ":" + buildID
	s.logger.Info("building labeled image", "tag", image, "version", version)
	if err := s.engine.BuildImage(ctx, ark.BuildRequest{
		Dockerfile: filepath.Join(contextDir, LocalLabelsDockerfilePath),
		Tag:        image,
		ExtraTags:  []string{DefaultLocalImageName + ":" + version, DefaultLocalImageName + ":latest"},
		BuildArgs: map[string]string{
			"BASE_IMAGE":         baseTag,
			"ARK_SERVER_VERSION": version,
			"ARK_BUILD_ID":       buildID,
		},
		Context: contextDir,
	}); err != nil {
		return BuildResult{}, fmt.Errorf("Error: labeled image build failed: %w", err)
	}
	if err := s.engine.RemoveImage(ctx, filesImage); err != nil {
		s.logger.Warn("could not remove the server files image", "image", filesImage, "error", err)
	}
	s.logger.Info("build complete", "tag", image)
	return BuildResult{Image: image, BuildID: buildID, Version: version, Built: true}, nil
}

// ensureBaseImage makes the base image for this recipe (arkcontainer.BaseImageRef)
// available: a local copy, else pulled from GHCR, else built here from
// base.dockerfile under the same name, so an update never depends on GHCR.
func (s *Service) ensureBaseImage(ctx context.Context, contextDir string) (string, error) {
	ref := arkcontainer.BaseImageRef()
	exists, err := s.engine.ImageExists(ctx, ref)
	if err != nil || exists {
		return ref, err
	}
	pullErr := s.engine.PullImage(ctx, ref)
	if pullErr == nil {
		return ref, nil
	}
	s.logger.Warn("could not pull the base image; building it locally", "image", ref, "error", pullErr)
	if err := s.engine.BuildImage(ctx, ark.BuildRequest{
		Dockerfile: filepath.Join(contextDir, "base.dockerfile"),
		Tag:        ref,
		Context:    contextDir,
	}); err != nil {
		return "", fmt.Errorf("Error: base image build failed: %w", err)
	}
	return ref, nil
}

// checkBuildDisk requires ARK_BUILD_REQUIRED_FREE_GB (default 50) GiB free at stateDir.
func (s *Service) checkBuildDisk(stateDir string) error {
	raw := envOrDefault("ARK_BUILD_REQUIRED_FREE_GB", "50")
	required, err := strconv.Atoi(raw)
	if err != nil || required <= 0 {
		return fmt.Errorf("Error: ARK_BUILD_REQUIRED_FREE_GB must be a positive whole number, got %q", raw)
	}
	free, _, err := s.DiskUsage(stateDir)
	if err != nil {
		return fmt.Errorf("Error: could not check free disk in %s: %w", stateDir, err)
	}
	const gib = 1 << 30
	if free < int64(required)*gib {
		return fmt.Errorf("Error: building the ARK image needs about %d GB free in %s (%d GB available); set ARK_BUILD_REQUIRED_FREE_GB to override", required, stateDir, free/gib)
	}
	return nil
}

// EnsureLocalImages makes each targeted service run an existing
// ark-server:<build ID> image and pins compose to it:
//   - ark-server or ark-server:latest: pin to ark-server:latest's build-ID tag,
//     building first if ark-server:latest doesn't exist;
//   - ark-server:<tag>: keep it; if it's missing, build it when <tag> is Steam's
//     current build, otherwise ask for an update;
//   - any other image is an error: only locally built images are supported.
//
// Pins are applied to project in place and saved once.
func (s *Service) EnsureLocalImages(ctx context.Context, composeFile string, project *ark.ComposeProject, services []string) error {
	if project == nil || project.Project == nil || len(project.Project.Services) == 0 {
		return errors.New("Error: compose config has no services to start.")
	}
	pins := map[string]string{}
	for _, name := range services {
		svc, ok := project.Project.Services[name]
		if !ok {
			continue
		}
		image, err := s.ensureServiceImage(ctx, composeFile, name, svc.Image)
		if err != nil {
			return err
		}
		if image != svc.Image {
			pins[name] = image
		}
	}
	if len(pins) == 0 {
		return nil
	}
	for name, image := range pins {
		if err := composeutil.MutateService(project.Project, name, func(svc *types.ServiceConfig) { svc.Image = image }); err != nil {
			return err
		}
		s.logger.Info("pinned service image", "service", name, "image", image)
	}
	return s.compose.SaveProject(ctx, composeFile, project)
}

// ensureServiceImage returns the ark-server:<build ID> image a service must run.
func (s *Service) ensureServiceImage(ctx context.Context, composeFile, service, image string) (string, error) {
	repo, tag, _ := strings.Cut(strings.TrimSpace(image), ":")
	if repo != DefaultLocalImageName {
		return "", fmt.Errorf("Error: %s uses image %q; only locally built %s images are supported", service, image, DefaultLocalImageName)
	}
	if tag == "" || tag == "latest" {
		latest := DefaultLocalImageName + ":latest"
		exists, err := s.engine.ImageExists(ctx, latest)
		if err != nil {
			return "", err
		}
		if !exists {
			built, err := s.BuildLocalImage(ctx, StateDir(composeFile))
			return built.Image, err
		}
		info, err := s.InspectImageVersionInfo(ctx, latest)
		if err != nil {
			return "", err
		}
		if info.BuildID == "" {
			return "", fmt.Errorf("Error: image %s has no %s label; rebuild with: ark-cli update instance --instance %s --rollout", latest, LabelARKBuildID, service)
		}
		return DefaultLocalImageName + ":" + info.BuildID, nil
	}
	exists, err := s.engine.ImageExists(ctx, image)
	if err != nil || exists {
		return image, err
	}
	steamBuildID, err := s.FetchSteamLatestBuildID()
	if err != nil || steamBuildID != tag {
		return "", fmt.Errorf("Error: image %s for %s is missing; run: ark-cli update instance --instance %s --rollout", image, service, service)
	}
	built, err := s.BuildLocalImage(ctx, StateDir(composeFile))
	return built.Image, err
}
