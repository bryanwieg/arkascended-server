package compose

import (
	"ark-cli/internal/core"

	"pkg/ark/update"
)

// RemoveOldImages drops ARK images that neither the compose services nor any
// instance's previous image (kept for rollback) reference, once no container
// uses them. A failure only warns: the command it follows has already succeeded.
func RemoveOldImages(d *core.AppDeps, composeFile string) {
	if d.UpdateService == nil || d.Engine == nil {
		return // not wired (CLI tests that don't use images)
	}
	project, err := d.ComposeService.LoadProject(d.Context(), composeFile)
	if err != nil {
		core.Warnf("Warning: could not check for old images: %v\n", err)
		return
	}
	// A corrupt state file must never let cleanup delete an instance's rollback image.
	states, err := update.LoadAllStates(update.StateDir(composeFile))
	if err != nil {
		core.Warnf("Warning: could not read update state, skipping image cleanup: %v\n", err)
		return
	}
	var keep []string
	for _, svc := range project.Project.Services {
		if svc.Image != "" {
			keep = append(keep, svc.Image)
		}
	}
	for _, st := range states {
		if st.PreviousImage != "" {
			keep = append(keep, st.PreviousImage)
		}
	}
	removed, err := d.UpdateService.RemoveOldImages(d.Context(), keep)
	for _, ref := range removed {
		core.Infof("Removed old image %s\n", ref)
	}
	if err != nil {
		core.Warnf("Warning: could not remove some old images: %v\n", err)
	}
}
