package update

import (
	"context"
	"path/filepath"
	"strings"
	"testing"

	"github.com/compose-spec/compose-go/v2/types"

	"pkg/ark"
	"pkg/ark/arktest"
)

func TestEnsureLocalImages(t *testing.T) {
	for _, tc := range []struct {
		name      string
		image     string
		exists    map[string]bool
		wantImage string // compose image afterwards
		wantSave  bool
		wantBuild bool
		wantErr   string
	}{
		{name: "latest built: pin", image: "ark-server:latest", exists: map[string]bool{"ark-server:latest": true}, wantImage: LocalImageRef("200"), wantSave: true},
		{name: "untagged built: pin", image: "ark-server", exists: map[string]bool{"ark-server:latest": true}, wantImage: LocalImageRef("200"), wantSave: true},
		{name: "latest missing: build and pin", image: "ark-server:latest", exists: map[string]bool{}, wantImage: LocalImageRef("200"), wantSave: true, wantBuild: true},
		{name: "pinned exists: unchanged", image: "ark-server:100", exists: map[string]bool{"ark-server:100": true}, wantImage: "ark-server:100"},
		{name: "pinned missing, Steam current: build", image: LocalImageRef("200"), exists: map[string]bool{}, wantImage: LocalImageRef("200"), wantBuild: true},
		{name: "pinned missing, old recipe: error", image: "ark-server:200", exists: map[string]bool{}, wantErr: "image ark-server:200 for asa1 is missing"},
		{name: "pinned missing, old build: error", image: "ark-server:100", exists: map[string]bool{}, wantErr: "image ark-server:100 for asa1 is missing; run: ark-cli update instance --instance asa1 --rollout"},
		{name: "other image: error", image: "arkserver/arkascended-server:200", wantErr: "only locally built ark-server images are supported"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			r := newBuildRig()
			r.exists = tc.exists
			s := r.service()
			saved := false
			s.compose = &arktest.MockCompose{
				SaveProjectFn: func(context.Context, string, *ark.ComposeProject) error {
					saved = true
					return nil
				},
			}
			project := &ark.ComposeProject{Project: &types.Project{Services: types.Services{"asa1": {Name: "asa1", Image: tc.image}}}}
			composeFile := filepath.Join(t.TempDir(), "docker-compose.yaml")

			err := s.EnsureLocalImages(context.Background(), composeFile, project, []string{"asa1"})
			if tc.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tc.wantErr) {
					t.Fatalf("err = %v, want %q", err, tc.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if got := project.Project.Services["asa1"].Image; got != tc.wantImage {
				t.Fatalf("image %q, want %q", got, tc.wantImage)
			}
			if saved != tc.wantSave {
				t.Fatalf("saved = %v, want %v", saved, tc.wantSave)
			}
			if built := len(r.builds) > 0; built != tc.wantBuild {
				t.Fatalf("built = %v, want %v", built, tc.wantBuild)
			}
		})
	}
}
