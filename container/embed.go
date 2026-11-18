// Package arkcontainer embeds the ARK server image build recipe so ark-cli can
// build the image on the host without a repository checkout.
package arkcontainer

import (
	"crypto/sha256"
	"embed"
	"encoding/hex"
	"fmt"
	"io/fs"
	"os"
	"path"
	"path/filepath"
)

// FS holds the image build recipe. all: keeps _-prefixed files such as
// scripts/runtime/_logging.py, which go:embed drops by default.
//
//go:embed base.dockerfile dockerfile dockerfile.labels .dockerignore all:scripts files
var FS embed.FS

// Extract writes FS to dir, replacing dir's previous contents. Scripts (*.sh,
// *.py) are written 0755, other files 0644; __pycache__ folders are skipped.
func Extract(dir string) error {
	if err := os.RemoveAll(dir); err != nil {
		return err
	}
	return fs.WalkDir(FS, ".", func(name string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() {
			if entry.Name() == "__pycache__" {
				return fs.SkipDir
			}
			return os.MkdirAll(filepath.Join(dir, filepath.FromSlash(name)), 0o755)
		}
		data, err := FS.ReadFile(name)
		if err != nil {
			return err
		}
		mode := os.FileMode(0o644)
		if ext := path.Ext(name); ext == ".sh" || ext == ".py" {
			mode = 0o755
		}
		return os.WriteFile(filepath.Join(dir, filepath.FromSlash(name)), data, mode)
	})
}

// BaseImageRepo is where CI publishes the base image (base.dockerfile).
const BaseImageRepo = "ghcr.io/bryanwieg/ark-server-base"

// BaseImageRef returns BaseImageRepo tagged with the first 12 hex characters of
// sha256(base.dockerfile), so each recipe maps to exactly one base image.
func BaseImageRef() string {
	data, err := FS.ReadFile("base.dockerfile")
	if err != nil {
		panic(err) // embedded at build time; can't be missing
	}
	sum := sha256.Sum256(data)
	return BaseImageRepo + ":" + hex.EncodeToString(sum[:])[:12]
}

// RecipeID returns the first 12 hex characters of a hash over every embedded recipe
// file (path and contents). Images are tagged with it, so an ark-cli whose scripts or
// Dockerfiles changed builds a new image instead of reusing one for the same ARK build.
func RecipeID() string {
	h := sha256.New()
	err := fs.WalkDir(FS, ".", func(name string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() {
			if entry.Name() == "__pycache__" { // local bytecode; Extract skips it too
				return fs.SkipDir
			}
			return nil
		}
		data, err := FS.ReadFile(name)
		if err != nil {
			return err
		}
		fmt.Fprintf(h, "%s\x00%d\x00", name, len(data))
		h.Write(data)
		return nil
	})
	if err != nil {
		panic(err) // embedded at build time; can't be unreadable
	}
	return hex.EncodeToString(h.Sum(nil))[:12]
}
