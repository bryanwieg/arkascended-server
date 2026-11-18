package update

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"testing"

	arkcontainer "ark-container"
	"pkg/ark"
	"pkg/ark/arktest"
)

// fakeImages is a local image store: repository -> tag reference -> image ID.
type fakeImages struct {
	tags      map[string]map[string]string
	labels    map[string]map[string]string // image reference -> labels
	inUse     map[string]bool              // image IDs a container uses
	inUseErr  error
	pruneErr  error
	listed    []string
	removedID []string
}

func (f *fakeImages) engine() *arktest.MockEngine {
	return &arktest.MockEngine{
		ImageLabelsFn: func(_ context.Context, ref string) (map[string]string, error) {
			if l, ok := f.labels[ref]; ok {
				return l, nil
			}
			return nil, fmt.Errorf("image %q: %w", ref, ark.ErrNotFound)
		},
		ImageTagsFn: func(_ context.Context, repo string) (map[string]string, error) {
			f.listed = append(f.listed, repo)
			out := map[string]string{}
			for ref, id := range f.tags[repo] {
				out[ref] = id
			}
			return out, nil
		},
		RemoveImageFn: func(_ context.Context, ref string) error {
			for repo, tags := range f.tags {
				id, ok := tags[ref]
				if !ok {
					continue
				}
				last := true
				for other, otherID := range tags {
					if other != ref && otherID == id {
						last = false
					}
				}
				if last && f.inUse[id] {
					return fmt.Errorf("image %q: %w", ref, ark.ErrConflict)
				}
				delete(f.tags[repo], ref)
				f.removedID = append(f.removedID, id)
				return nil
			}
			return fmt.Errorf("image %q: %w", ref, ark.ErrNotFound)
		},
		ImagesInUseFn:   func(context.Context) (map[string]bool, error) { return f.inUse, f.inUseErr },
		PruneDanglingFn: func(context.Context) error { return f.pruneErr },
	}
}

const filesRepo = FilesImageRepo
const baseRepo = arkcontainer.BaseImageRepo

var currentBase = arkcontainer.BaseImageRef()

func arkLabels(buildID string) map[string]string {
	return map[string]string{"io.ark.server.build_id": buildID}
}

func TestRemoveOldImagesKeepsOnlyTheCurrentBuildAndNoFilesImages(t *testing.T) {
	f := &fakeImages{
		tags: map[string]map[string]string{
			"ark-server": {
				"ark-server:latest": "new", "ark-server:98.6": "new", "ark-server:200": "new",
				"ark-server:98.5": "old", "ark-server:100": "old",
			},
			filesRepo: {filesRepo + ":200": "fnew", filesRepo + ":latest": "fnew", filesRepo + ":100": "fold"},
			baseRepo:  {currentBase: "bnew", baseRepo + ":0123456789ab": "bold"},
		},
		labels: map[string]map[string]string{"ark-server:latest": arkLabels("200")},
	}
	removed, err := testService(f.engine()).RemoveOldImages(context.Background(), []string{"ark-server"})
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"ark-server:100", "ark-server:98.5", filesRepo + ":100", filesRepo + ":200", filesRepo + ":latest", baseRepo + ":0123456789ab"}
	if !slices.Equal(removed, want) {
		t.Fatalf("removed %v, want %v", removed, want)
	}
	if len(f.tags["ark-server"]) != 3 || len(f.tags[filesRepo]) != 0 || len(f.tags[baseRepo]) != 1 {
		t.Fatalf("current images must stay, left: %v", f.tags)
	}
}

func TestRemoveOldImagesKeepsImagesAContainerUses(t *testing.T) {
	// An instance not restarted yet, or another compose project's stopped container,
	// still uses the old image: none of its tags are removed.
	f := &fakeImages{
		tags: map[string]map[string]string{
			"arkserver/arkascended-server": {
				"arkserver/arkascended-server:200": "new",
				"arkserver/arkascended-server:100": "old", "arkserver/arkascended-server:98.5": "old",
			},
		},
		labels: map[string]map[string]string{"arkserver/arkascended-server:200": arkLabels("200")},
		inUse:  map[string]bool{"old": true},
	}
	removed, err := testService(f.engine()).RemoveOldImages(context.Background(), []string{"docker.io/arkserver/arkascended-server:200"})
	if err != nil {
		t.Fatalf("an image in use is skipped, not an error: %v", err)
	}
	if len(removed) != 0 || len(f.tags["arkserver/arkascended-server"]) != 3 {
		t.Fatalf("expected every tag kept, removed %v, left %v", removed, f.tags)
	}
	if slices.Contains(f.removedID, "new") {
		t.Fatal("the current image must not be touched")
	}
}

func TestRemoveOldImagesRemovesNothingWhenContainersCannotBeListed(t *testing.T) {
	f := &fakeImages{
		tags:     map[string]map[string]string{"ark-server": {"ark-server:latest": "new", "ark-server:100": "old"}},
		labels:   map[string]map[string]string{"ark-server:latest": arkLabels("200")},
		inUseErr: errors.New("daemon unavailable"),
	}
	removed, err := testService(f.engine()).RemoveOldImages(context.Background(), []string{"ark-server:latest"})
	if err == nil || len(removed) != 0 || len(f.tags["ark-server"]) != 2 {
		t.Fatalf("expected an error and nothing removed, got removed %v, err %v", removed, err)
	}
}

func TestRemoveOldImagesLeavesNonArkAndPinnedRepositoriesAlone(t *testing.T) {
	f := &fakeImages{
		tags: map[string]map[string]string{
			"nginx":      {"nginx:1.25": "a", "nginx:1.24": "b"},
			"ark-pinned": {"ark-pinned:1": "x", "ark-pinned:2": "y"},
		},
		labels: map[string]map[string]string{"nginx:1.25": {}},
	}
	removed, err := testService(f.engine()).RemoveOldImages(context.Background(),
		[]string{"nginx:1.25", "ark-pinned:2@sha256:0123"})
	if err != nil || len(removed) != 0 || len(f.listed) != 0 {
		t.Fatalf("expected nothing listed or removed, got removed %v listed %v err %v", removed, f.listed, err)
	}
}

func TestRemoveOldImagesReportsPruneErrorsAfterRemoving(t *testing.T) {
	f := &fakeImages{
		tags:     map[string]map[string]string{"ark-server": {"ark-server:latest": "new", "ark-server:100": "old"}},
		labels:   map[string]map[string]string{"ark-server:latest": arkLabels("200")},
		pruneErr: errors.New("prune failed"),
	}
	removed, err := testService(f.engine()).RemoveOldImages(context.Background(), []string{"ark-server:latest"})
	if !slices.Equal(removed, []string{"ark-server:100"}) || err == nil {
		t.Fatalf("expected the removal plus the prune error, got %v, %v", removed, err)
	}
}

func TestSplitImageRef(t *testing.T) {
	cases := []struct{ ref, repo, normalized string }{
		{"ark-server", "ark-server", "ark-server:latest"},
		{"ark-server:98.6", "ark-server", "ark-server:98.6"},
		{"docker.io/arkserver/arkascended-server:200", "arkserver/arkascended-server", "arkserver/arkascended-server:200"},
		{"localhost:5000/ark-server", "localhost:5000/ark-server", "localhost:5000/ark-server:latest"},
		{"localhost:5000/ark-server:1", "localhost:5000/ark-server", "localhost:5000/ark-server:1"},
	}
	for _, c := range cases {
		repo, normalized, ok := splitImageRef(c.ref)
		if !ok || repo != c.repo || normalized != c.normalized {
			t.Errorf("splitImageRef(%q) = %q, %q, %v; want %q, %q", c.ref, repo, normalized, ok, c.repo, c.normalized)
		}
	}
	if repo, _, ok := splitImageRef("ark-server:1@sha256:abc"); ok || repo != "ark-server" {
		t.Errorf("digest ref: got repo %q ok %v", repo, ok)
	}
}
