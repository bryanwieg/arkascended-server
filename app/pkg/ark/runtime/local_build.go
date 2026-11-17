package runtime

import "pkg/ark"

// EnsureLocalImages ensures local images exist for the given services.
func (h *RuntimeHelper) EnsureLocalImages(composeFile string, project *ark.ComposeProject, services []string) error {
	return h.UpdateService.EnsureLocalImages(h.Ctx, composeFile, project, services)
}
