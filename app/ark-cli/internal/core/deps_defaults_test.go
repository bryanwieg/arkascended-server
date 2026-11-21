package core

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"pkg/ark"
	"pkg/ark/arktest"
)

// TestCheckFileInContainerUsesEngineCopy checks the default ready-file probe
// copies through the engine API and never shells out to a runtime binary.
func TestCheckFileInContainerUsesEngineCopy(t *testing.T) {
	for _, tc := range []struct {
		name    string
		copyErr error
		want    bool
	}{
		{"copy succeeds", nil, true},
		{"copy fails", errors.New("no such file"), false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			tmp := t.TempDir()
			t.Setenv("ARK_TMP_DIR", tmp)
			var gotContainer, gotSrc, gotDst string
			d := &AppDeps{
				Ctx: context.Background(),
				Engine: &arktest.MockEngine{
					CopyFromContainerFn: func(_ context.Context, container, src, dst string) error {
						gotContainer, gotSrc, gotDst = container, src, dst
						return tc.copyErr
					},
				},
				ComposeService: &arktest.MockCompose{},
				Runtime: RuntimeDeps{
					RunCommand: func(cmd []string, _ ark.CommandOptions) (ark.ExecResult, error) {
						t.Fatalf("RunCommand called: %v", cmd)
						return ark.ExecResult{}, nil
					},
				},
			}
			FillDefaults(d)

			ok, err := d.BackupService.CheckFileInContainer(context.Background(), "helper", "/data/.ready")
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if ok != tc.want {
				t.Fatalf("got %v, want %v", ok, tc.want)
			}
			if gotContainer != "helper" || gotSrc != "/data/.ready" || filepath.Base(gotDst) != "ready" {
				t.Fatalf("unexpected copy args: %q %q %q", gotContainer, gotSrc, gotDst)
			}
			if entries, _ := os.ReadDir(tmp); len(entries) != 0 {
				t.Fatalf("temp dir not removed: %v", entries)
			}
		})
	}
}
