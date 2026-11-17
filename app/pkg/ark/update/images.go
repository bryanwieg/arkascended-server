package update

import (
	"context"
	"errors"
	"fmt"
	"maps"
	"slices"
	"strings"

	arkcontainer "ark-container"
	"pkg/ark"
)

// RemoveOldImages removes every local ARK server image except the ones keep refers
// to, every server-files image (only needed during a build), base image tags other
// than the current recipe's, then dangling images and unused build cache. Only repositories of
// ARK images (labelled io.ark.server.build_id) are touched; other services' images
// are left alone. Images a container
// uses are never deleted (see ark.ContainerEngine.RemoveImage). Returns the removed
// references; errors are collected so one failure doesn't stop the rest.
func (s *Service) RemoveOldImages(ctx context.Context, keep []string) ([]string, error) {
	keepRefs := map[string]bool{}
	repos := []string{}
	pinned := map[string]bool{}
	for _, ref := range keep {
		repo, normalized, ok := splitImageRef(ref)
		if !ok {
			pinned[repo] = true // a digest pin can't be matched to tags; leave its repository alone
			continue
		}
		labels, err := s.engine.ImageLabels(ctx, normalized)
		buildID := labels["io.ark.server.build_id"]
		if err != nil || buildID == "" {
			continue // not an ARK image, or not pulled yet (cleaned after the next start)
		}
		keepRefs[normalized] = true
		if !slices.Contains(repos, repo) {
			repos = append(repos, repo)
		}
	}
	repos = slices.DeleteFunc(repos, func(r string) bool { return pinned[r] })
	if len(repos) == 0 {
		return nil, nil
	}
	repos = append(repos, FilesImageRepo, arkcontainer.BaseImageRepo)
	keepRefs[arkcontainer.BaseImageRef()] = true // older base tags only untag: kept images still use their layers

	var removed []string
	var errs []error
	for _, repo := range repos {
		tags, err := s.engine.ImageTags(ctx, repo)
		if err != nil {
			errs = append(errs, fmt.Errorf("list %s images: %w", repo, err))
			continue
		}
		keepIDs := map[string]bool{}
		for ref, id := range tags {
			if keepRefs[ref] {
				keepIDs[id] = true
			}
		}
		for _, ref := range slices.Sorted(maps.Keys(tags)) {
			if keepIDs[tags[ref]] {
				continue
			}
			err := s.engine.RemoveImage(ctx, ref)
			switch {
			case err == nil:
				removed = append(removed, ref)
			case errors.Is(err, ark.ErrConflict), errors.Is(err, ark.ErrNotFound):
				// In use by a container (goes once that container is recreated) or already gone.
			default:
				errs = append(errs, err)
			}
		}
	}
	if err := s.engine.PruneDangling(ctx); err != nil {
		errs = append(errs, fmt.Errorf("prune dangling images and build cache: %w", err))
	}
	return removed, errors.Join(errs...)
}

// splitImageRef returns an image reference's repository and its "repo:tag" form as
// image listings show it (no docker.io/ prefix, ":latest" when untagged). ok is false
// for digest references.
func splitImageRef(ref string) (repo, normalized string, ok bool) {
	ref = strings.TrimPrefix(strings.TrimSpace(ref), "docker.io/")
	if name, _, isDigest := strings.Cut(ref, "@"); isDigest {
		repo, _, _ := splitImageRef(name)
		return repo, "", false
	}
	if i := strings.LastIndex(ref, ":"); i > strings.LastIndex(ref, "/") {
		return ref[:i], ref, true
	}
	return ref, ref + ":latest", true
}
