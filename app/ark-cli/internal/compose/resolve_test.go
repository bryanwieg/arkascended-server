package compose

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"ark-cli/internal/core"

	"pkg/ark"
	"pkg/ark/arktest"
)

// TestWriteComposeTextKeepsFileWhenEditFailsToLoad checks the load gate before the rename.
// - Expect: a failing load or check leaves the file and no temp copy; a passing one replaces the file.
func TestWriteComposeTextKeepsFileWhenEditFailsToLoad(t *testing.T) {
	composeFile := filepath.Join(t.TempDir(), "docker-compose.yaml")
	const original = "services:\n  asa1:\n    image: ark-server:latest\n"
	if err := os.WriteFile(composeFile, []byte(original), 0o600); err != nil {
		t.Fatal(err)
	}
	loadErr := errors.New("services must be a mapping")
	d := &core.AppDeps{ComposeService: &arktest.MockCompose{
		LoadProjectFn: func(context.Context, string) (*ark.ComposeProject, error) { return &ark.ComposeProject{}, loadErr },
	}}
	failCheck := func(*ark.ComposeProject) error { return errors.New("check failed") }

	if err := WriteComposeText(d, composeFile, "services: null\n", nil); !errors.Is(err, loadErr) {
		t.Fatalf("got %v, want the load error", err)
	}
	d.ComposeService = &arktest.MockCompose{}
	if err := WriteComposeText(d, composeFile, "services: {}\n", failCheck); err == nil {
		t.Fatal("expected the check error")
	}
	if got, _ := os.ReadFile(composeFile); string(got) != original {
		t.Fatalf("file changed after failed writes:\n%s", got)
	}
	if entries, _ := os.ReadDir(filepath.Dir(composeFile)); len(entries) != 1 {
		t.Fatalf("temp copy left behind: %v", entries)
	}

	if err := WriteComposeText(d, composeFile, "services: {}\n", nil); err != nil {
		t.Fatal(err)
	}
	info, _ := os.Stat(composeFile)
	if got, _ := os.ReadFile(composeFile); string(got) != "services: {}\n" || info.Mode().Perm() != 0o600 {
		t.Fatalf("got %q mode %v, want the new text with mode 0600 kept", got, info.Mode().Perm())
	}
}
