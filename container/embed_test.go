package arkcontainer

import (
	"bufio"
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestExtractWritesRecipe(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "context")
	if err := Extract(dir); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{
		"dockerfile",
		"base.dockerfile",
		"files/build.sh",
		"scripts/runtime/container-entrypoint.py",
		"scripts/runtime/_logging.py",
	} {
		want, err := os.ReadFile(name)
		if err != nil {
			t.Fatal(err)
		}
		got, err := os.ReadFile(filepath.Join(dir, name))
		if err != nil {
			t.Fatalf("%s not extracted: %v", name, err)
		}
		if !bytes.Equal(got, want) {
			t.Errorf("%s differs from the source file", name)
		}
	}
	info, err := os.Stat(filepath.Join(dir, "files/build.sh"))
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm()&0o100 == 0 {
		t.Errorf("files/build.sh mode %v, want executable", info.Mode().Perm())
	}
	err = filepath.WalkDir(dir, func(path string, _ os.DirEntry, err error) error {
		if strings.Contains(path, "__pycache__") {
			t.Errorf("extracted %s", path)
		}
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
}

func TestExtractReplacesPreviousContents(t *testing.T) {
	dir := t.TempDir()
	stray := filepath.Join(dir, "stray.txt")
	if err := os.WriteFile(stray, []byte("old"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := Extract(dir); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(stray); !os.IsNotExist(err) {
		t.Errorf("stray file survived Extract: %v", err)
	}
}

func TestBaseImageRefHashesBaseDockerfile(t *testing.T) {
	data, err := os.ReadFile("base.dockerfile")
	if err != nil {
		t.Fatal(err)
	}
	sum := sha256.Sum256(data)
	want := BaseImageRepo + ":" + hex.EncodeToString(sum[:])[:12]
	if got := BaseImageRef(); got != want {
		t.Fatalf("BaseImageRef() = %q, want %q", got, want)
	}
}

// The tag only covers base.dockerfile, so it must not copy files from the build context.
func TestBaseDockerfileCopiesNothingFromContext(t *testing.T) {
	f, err := os.Open("base.dockerfile")
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	scanner := bufio.NewScanner(f)
	for line := 1; scanner.Scan(); line++ {
		fields := strings.Fields(scanner.Text())
		if len(fields) > 0 && (fields[0] == "COPY" || fields[0] == "ADD") && !strings.Contains(scanner.Text(), "--from=") {
			t.Errorf("base.dockerfile:%d copies from the build context: %s", line, scanner.Text())
		}
	}
}
