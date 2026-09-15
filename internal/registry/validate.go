package registry

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"regexp"
	"sort"
	"strings"
)

// TagPattern is the release tag rule: vX.Y.Z, nothing else.
var TagPattern = regexp.MustCompile(`^v\d+\.\d+\.\d+$`)

// Validated is a plugin repository that passed every check, with everything
// the index builder needs from it.
type Validated struct {
	Repo     string // owner/name
	Owner    string
	Name     string // repository name (not the plugin name)
	Manifest Manifest
	Release  Release   // the latest published, non-prerelease release
	Version  string    // Release.Tag without the leading v
	Releases []Release // all published, non-prerelease releases, newest first
	Entry    []byte    // the plugin source at Release.Tag
	SHA256   string    // hex sha256 of Entry
}

// ValidateEntry checks one registry entry end to end: a published release
// tagged vX.Y.Z, a valid manifest and README at that tag, an entry file that
// loads in goblog, and an identity that matches the manifest and the tag.
func ValidateEntry(ctx context.Context, src Source, val Validator, repo string) (*Validated, error) {
	owner, name, ok := strings.Cut(repo, "/")
	if !ok || owner == "" || name == "" || strings.Contains(name, "/") {
		return nil, fmt.Errorf("%q: repo must be owner/name", repo)
	}

	all, err := src.Releases(ctx, owner, name)
	if err != nil {
		return nil, err
	}
	var releases []Release
	for _, r := range all {
		if !r.Draft && !r.Prerelease {
			releases = append(releases, r)
		}
	}
	if len(releases) == 0 {
		return nil, fmt.Errorf("%s: no published release (drafts and pre-releases are ignored)", repo)
	}
	sort.SliceStable(releases, func(i, j int) bool { return releases[i].PublishedAt.After(releases[j].PublishedAt) })
	latest := releases[0]
	if !TagPattern.MatchString(latest.Tag) {
		return nil, fmt.Errorf("%s: release tag %q must be vX.Y.Z", repo, latest.Tag)
	}
	version := strings.TrimPrefix(latest.Tag, "v")

	// The "latest" check above still runs against the unfiltered list (so a
	// bad tag on the newest release is still an error); the history shown to
	// consumers drops anything that was never a valid vX.Y.Z tag, such as an
	// old release tagged before the convention was adopted.
	filtered := releases[:0:0]
	for _, r := range releases {
		if TagPattern.MatchString(r.Tag) {
			filtered = append(filtered, r)
		}
	}
	releases = filtered

	mb, err := src.File(ctx, owner, name, latest.Tag, "goblog-plugin.json")
	if err != nil {
		return nil, fmt.Errorf("%s@%s: goblog-plugin.json: %w", repo, latest.Tag, err)
	}
	manifest, err := ParseManifest(mb)
	if err != nil {
		return nil, fmt.Errorf("%s@%s: %w", repo, latest.Tag, err)
	}
	if _, err := src.File(ctx, owner, name, latest.Tag, "README.md"); err != nil {
		return nil, fmt.Errorf("%s@%s: README.md: %w", repo, latest.Tag, err)
	}
	entry, err := src.File(ctx, owner, name, latest.Tag, manifest.Entry)
	if err != nil {
		return nil, fmt.Errorf("%s@%s: entry %s: %w", repo, latest.Tag, manifest.Entry, err)
	}

	info, err := val.Validate(ctx, entry)
	if err != nil {
		return nil, fmt.Errorf("%s@%s: %s does not load: %w", repo, latest.Tag, manifest.Entry, err)
	}
	if info.Name != manifest.Name {
		return nil, fmt.Errorf("%s@%s: Name() is %q but the manifest says %q", repo, latest.Tag, info.Name, manifest.Name)
	}
	if info.Version != version {
		return nil, fmt.Errorf("%s@%s: Version() is %q but the release tag says %q", repo, latest.Tag, info.Version, version)
	}

	h := sha256.Sum256(entry)
	return &Validated{
		Repo: repo, Owner: owner, Name: name,
		Manifest: manifest, Release: latest, Version: version, Releases: releases,
		Entry: entry, SHA256: hex.EncodeToString(h[:]),
	}, nil
}

// isNotFound reports whether err is a missing-file error from a Source.
func isNotFound(err error) bool { return errors.Is(err, ErrNotFound) }
