package compose

import (
	"ark-cli/internal/core"
	"context"
	"errors"
	"fmt"
	"os"
	"strings"
	"testing"

	"github.com/compose-spec/compose-go/v2/types"

	"pkg/ark"
	"pkg/ark/arktest"
	"pkg/ark/update"
)

func TestRemoveOldImagesReportsRemovalsAndOnlyWarnsOnFailure(t *testing.T) {
	var stdout, stderr strings.Builder
	origInfo, origWarn := core.Infof, core.Warnf
	core.Infof = func(f string, a ...any) { fmt.Fprintf(&stdout, f, a...) }
	core.Warnf = func(f string, a ...any) { fmt.Fprintf(&stderr, f, a...) }
	t.Cleanup(func() { core.Infof, core.Warnf = origInfo, origWarn })

	withPatchedAppDeps(t, func(d *core.AppDeps) {
		d.ComposeService = &arktest.MockCompose{
			LoadProjectFn: func(context.Context, string) (*ark.ComposeProject, error) {
				return fakeProject(types.Services{"asa1": {Name: "asa1", Image: "ark-server:latest"}}), nil
			},
		}
		d.Engine = &arktest.MockEngine{
			ImageLabelsFn: func(context.Context, string) (map[string]string, error) {
				return map[string]string{"io.ark.server.build_id": "200"}, nil
			},
			ImageTagsFn: func(_ context.Context, repo string) (map[string]string, error) {
				if repo == "ark-server" {
					return map[string]string{"ark-server:latest": "new", "ark-server:100": "old"}, nil
				}
				return map[string]string{}, nil
			},
			PruneDanglingFn: func(context.Context) error { return errors.New("prune failed") },
		}
		d.UpdateService = update.New(d.Engine, d.ComposeService, nil, d.Logger)
	}, func() {
		RemoveOldImages(core.CurrentAppDeps(), "docker-compose.yaml")
	})

	if !strings.Contains(stdout.String(), "Removed old image ark-server:100") {
		t.Fatalf("expected the removal to be reported, got %q", stdout.String())
	}
	if !strings.Contains(stderr.String(), "could not remove some old images: prune dangling images and build cache: prune failed") {
		t.Fatalf("expected a warning for the failure, got %q", stderr.String())
	}
}

func TestRemoveOldImagesKeepsPreviousImagesAndSkipsOnCorruptState(t *testing.T) {
	composeFile := t.TempDir() + "/docker-compose.yaml"
	stateDir := update.StateDir(composeFile)
	if err := update.SaveState(stateDir, "asa1", update.InstanceState{PreviousImage: "ark-server:100"}); err != nil {
		t.Fatal(err)
	}
	var removed []string
	run := func() {
		withPatchedAppDeps(t, func(d *core.AppDeps) {
			d.ComposeService = &arktest.MockCompose{
				LoadProjectFn: func(context.Context, string) (*ark.ComposeProject, error) {
					return fakeProject(types.Services{"asa1": {Name: "asa1", Image: "ark-server:200"}}), nil
				},
			}
			d.Engine = &arktest.MockEngine{
				ImageLabelsFn: func(_ context.Context, ref string) (map[string]string, error) {
					return map[string]string{"io.ark.server.build_id": strings.TrimPrefix(ref, "ark-server:")}, nil
				},
				ImageTagsFn: func(_ context.Context, repo string) (map[string]string, error) {
					if repo == "ark-server" {
						return map[string]string{"ark-server:200": "new", "ark-server:100": "prev", "ark-server:50": "old"}, nil
					}
					return map[string]string{}, nil
				},
				RemoveImageFn: func(_ context.Context, ref string) error {
					removed = append(removed, ref)
					return nil
				},
			}
			d.UpdateService = update.New(d.Engine, d.ComposeService, nil, d.Logger)
		}, func() {
			RemoveOldImages(core.CurrentAppDeps(), composeFile)
		})
	}

	run()
	if strings.Join(removed, ",") != "ark-server:50" {
		t.Fatalf("removed %v, want only ark-server:50 (ark-server:100 is the rollback image)", removed)
	}

	removed = nil
	if err := os.WriteFile(update.StatePath(stateDir, "asa1"), []byte("{corrupt"), 0o644); err != nil {
		t.Fatal(err)
	}
	run()
	if len(removed) != 0 {
		t.Fatalf("removed %v with a corrupt state file, want no cleanup", removed)
	}
}
