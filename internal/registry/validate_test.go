package registry

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"
)

// memSource is an in-memory Source for validator/builder tests.
type memSource struct {
	releases map[string][]Release // "owner/repo" → releases
	files    map[string]string    // "owner/repo@ref:path" → content
	rendered int                  // RenderMarkdown call count
}

func (m *memSource) Releases(_ context.Context, owner, repo string) ([]Release, error) {
	rels, ok := m.releases[owner+"/"+repo]
	if !ok {
		return nil, errors.New("no such repo")
	}
	return rels, nil
}

func (m *memSource) File(_ context.Context, owner, repo, ref, path string) ([]byte, error) {
	if c, ok := m.files[owner+"/"+repo+"@"+ref+":"+path]; ok {
		return []byte(c), nil
	}
	return nil, ErrNotFound
}

func (m *memSource) RenderMarkdown(_ context.Context, ownerRepo, md string) (string, error) {
	if md == "" {
		return "", nil
	}
	m.rendered++
	return "<p>" + md + "</p>", nil
}

const helloSrc = "package main\n// hello plugin\n"

func helloSource() *memSource {
	return &memSource{
		releases: map[string][]Release{
			"o/hello": {
				{Tag: "v1.1.0", Body: "Second", URL: "https://github.com/o/hello/releases/tag/v1.1.0", PublishedAt: time.Date(2026, 9, 15, 0, 0, 0, 0, time.UTC)},
				{Tag: "v2.0.0-rc1", Prerelease: true, PublishedAt: time.Date(2026, 9, 16, 0, 0, 0, 0, time.UTC)},
				{Tag: "v3.0.0", Draft: true, PublishedAt: time.Date(2026, 9, 17, 0, 0, 0, 0, time.UTC)},
				{Tag: "v1.0.0", Body: "First", URL: "https://github.com/o/hello/releases/tag/v1.0.0", PublishedAt: time.Date(2026, 9, 14, 0, 0, 0, 0, time.UTC)},
			},
		},
		files: map[string]string{
			"o/hello@v1.1.0:goblog-plugin.json": goodManifest,
			"o/hello@v1.1.0:plugin.go":          helloSrc,
			"o/hello@v1.1.0:README.md":          "# Hello",
			"o/hello@v1.1.0:CHANGELOG.md":       "## 1.1.0\n- second",
		},
	}
}

func helloValidator() *FakeValidator {
	return &FakeValidator{Infos: map[string]Info{sum([]byte(helloSrc)): {Name: "hello", DisplayName: "Hello", Version: "1.1.0"}}}
}

func TestValidateEntry_Good(t *testing.T) {
	v, err := ValidateEntry(context.Background(), helloSource(), helloValidator(), "o/hello")
	if err != nil {
		t.Fatal(err)
	}
	if v.Owner != "o" || v.Name != "hello" || v.Manifest.Name != "hello" || v.Release.Tag != "v1.1.0" || v.Version != "1.1.0" {
		t.Errorf("validated = %+v", v)
	}
	if len(v.Releases) != 2 || v.Releases[0].Tag != "v1.1.0" || v.Releases[1].Tag != "v1.0.0" {
		t.Errorf("releases should exclude drafts/prereleases, newest first: %+v", v.Releases)
	}
	if string(v.Entry) != helloSrc || v.SHA256 != sum([]byte(helloSrc)) {
		t.Errorf("entry/sha mismatch")
	}
}

func TestValidateEntry_PicksLatestByDate(t *testing.T) {
	src := helloSource()
	// GitHub order is not trusted: put the older release first.
	rels := src.releases["o/hello"]
	src.releases["o/hello"] = []Release{rels[3], rels[0]}
	v, err := ValidateEntry(context.Background(), src, helloValidator(), "o/hello")
	if err != nil {
		t.Fatal(err)
	}
	if v.Release.Tag != "v1.1.0" {
		t.Errorf("latest should be v1.1.0 by published date, got %s", v.Release.Tag)
	}
}

func TestValidateEntry_Errors(t *testing.T) {
	type tc struct {
		mutate func(s *memSource, f *FakeValidator)
		want   string
	}
	cases := map[string]tc{
		"bad repo string": {func(s *memSource, f *FakeValidator) {}, "owner/name"},
		"no releases": {func(s *memSource, f *FakeValidator) {
			s.releases["o/hello"] = []Release{{Tag: "v9.0.0", Draft: true}}
		}, "no published release"},
		"tag not semver": {func(s *memSource, f *FakeValidator) {
			s.releases["o/hello"][0].Tag = "1.1.0"
			s.files["o/hello@1.1.0:goblog-plugin.json"] = goodManifest
		}, "vX.Y.Z"},
		"missing manifest": {func(s *memSource, f *FakeValidator) {
			delete(s.files, "o/hello@v1.1.0:goblog-plugin.json")
		}, "goblog-plugin.json"},
		"invalid manifest": {func(s *memSource, f *FakeValidator) {
			s.files["o/hello@v1.1.0:goblog-plugin.json"] = `{"name":"Bad"}`
		}, "goblog-plugin.json"},
		"missing entry": {func(s *memSource, f *FakeValidator) {
			delete(s.files, "o/hello@v1.1.0:plugin.go")
		}, "plugin.go"},
		"missing readme": {func(s *memSource, f *FakeValidator) {
			delete(s.files, "o/hello@v1.1.0:README.md")
		}, "README.md"},
		"does not load": {func(s *memSource, f *FakeValidator) {
			f.Err = errors.New("yaegi: boom")
		}, "boom"},
		"name mismatch": {func(s *memSource, f *FakeValidator) {
			f.Infos[sum([]byte(helloSrc))] = Info{Name: "other", Version: "1.1.0"}
		}, "Name()"},
		"version mismatch": {func(s *memSource, f *FakeValidator) {
			f.Infos[sum([]byte(helloSrc))] = Info{Name: "hello", Version: "1.0.9"}
		}, "Version()"},
	}
	for name, c := range cases {
		s, f := helloSource(), helloValidator()
		c.mutate(s, f)
		repo := "o/hello"
		if name == "bad repo string" {
			repo = "hello"
		}
		_, err := ValidateEntry(context.Background(), s, f, repo)
		if err == nil || !strings.Contains(err.Error(), c.want) {
			t.Errorf("%s: want error containing %q, got %v", name, c.want, err)
		}
	}
}
