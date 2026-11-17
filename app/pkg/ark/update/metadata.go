package update

import (
	"context"
	"fmt"
	"regexp"
	"strings"
)

const (
	LabelOCIImageVersion = "org.opencontainers.image.version"
	LabelOCIImageDesc    = "org.opencontainers.image.description"
	LabelARKBuildID      = "io.ark.server.build_id"
	LabelARKVersion      = "io.ark.server.version"

	ContainerVersionPath      = "/home/ark-user/ARK_SERVER_VERSION"
	ContainerBuildIDPath      = "/home/ark-user/ARK_SERVER_BUILDID"
	LocalDockerfilePath       = "dockerfile"
	LocalLabelsDockerfilePath = "dockerfile.labels"
	DefaultLocalImageName     = "ark-server"
	LocalBaseTagSuffix        = "base"
)

// InspectImageVersionInfo reads version metadata from image labels.
func (s *Service) InspectImageVersionInfo(ctx context.Context, image string) (ImageVersionInfo, error) {
	if s.engine == nil {
		return ImageVersionInfo{Image: image}, fmt.Errorf("engine not initialized")
	}
	labels, err := s.engine.ImageLabels(ctx, image)
	if err != nil {
		return ImageVersionInfo{Image: image}, err
	}
	version := strings.TrimSpace(FirstNonEmpty(labels[LabelARKVersion], labels[LabelOCIImageVersion]))
	buildID := strings.TrimSpace(labels[LabelARKBuildID])
	if buildID == "" {
		buildID = ExtractBuildIDFromDescription(labels[LabelOCIImageDesc])
	}
	info := ImageVersionInfo{
		Image:   image,
		Version: version,
		BuildID: buildID,
	}
	return info, nil
}

// ExtractBuildIDFromDescription extracts a build ID from an OCI description label.
func ExtractBuildIDFromDescription(value string) string {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return ""
	}
	match := regexp.MustCompile(`(?i)build\s*id\s*:\s*([0-9]+)`).FindStringSubmatch(trimmed)
	if len(match) < 2 {
		return ""
	}
	return match[1]
}

// FirstNonEmpty returns the first non-whitespace string from the arguments.
func FirstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}
