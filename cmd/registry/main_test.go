package main

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/goblogplatform/plugins/internal/registry"
)

type memSource struct {
	releases map[string][]registry.Release
	files    map[string]string
}

func (m *memSource) Releases(_ context.Context, owner, repo string) ([]registry.Release, error) {
	if r, ok := m.releases[owner+"/"+repo]; ok {
		return r, nil
	}
	return nil, errors.New("no such repo")
}
func (m *memSource) File(_ context.Context, owner, repo, ref, path string) ([]byte, error) {
	if c, ok := m.files[owner+"/"+repo+"@"+ref+":"+path]; ok {
		return []byte(c), nil
	}
	return nil, registry.ErrNotFound
}
func (m *memSource) RenderMarkdown(_ context.Context, _, md string) (string, error) {
	return "<p>" + md + "</p>", nil
}
func (m *memSource) RepoStars(context.Context, string, string) (int, error) { return 3, nil }

type okValidator struct{}

func (okValidator) Validate(_ context.Context, _ []byte) (registry.Info, error) {
	return registry.Info{Name: "hello", DisplayName: "Hello", Version: "1.0.0"}, nil
}

func fixture(t *testing.T) (string, *memSource) {
	t.Helper()
	dir := t.TempDir()
	reg := filepath.Join(dir, "registry.yaml")
	os.WriteFile(reg, []byte("plugins:\n  - repo: o/hello\n  - repo: o/broken\n"), 0644)
	src := &memSource{
		releases: map[string][]registry.Release{
			"o/hello":  {{Tag: "v1.0.0", Body: "First", URL: "u", PublishedAt: time.Date(2026, 9, 14, 0, 0, 0, 0, time.UTC)}},
			"o/broken": {},
		},
		files: map[string]string{
			"o/hello@v1.0.0:goblog-plugin.json": `{"name":"hello","display_name":"Hello","description":"d","author":"a","license":"MIT","min_goblog_version":"0.2.6"}`,
			"o/hello@v1.0.0:plugin.go":          "package main\n",
			"o/hello@v1.0.0:README.md":          "# Hello",
		},
	}
	return reg, src
}

func okFactory(string) registry.Validator { return okValidator{} }

func TestRun_Validate(t *testing.T) {
	reg, src := fixture(t)
	var out, errOut bytes.Buffer
	code := run([]string{"validate", "--registry", reg, "--repo", "o/hello"}, &out, &errOut, src, okFactory)
	if code != 0 || !strings.Contains(out.String(), "o/hello: ok (hello 1.0.0)") {
		t.Errorf("code=%d out=%q err=%q", code, out.String(), errOut.String())
	}
	out.Reset()
	errOut.Reset()
	code = run([]string{"validate", "--registry", reg}, &out, &errOut, src, okFactory)
	if code != 1 || !strings.Contains(errOut.String(), "o/broken") || !strings.Contains(out.String(), "o/hello: ok") {
		t.Errorf("all entries: code=%d out=%q err=%q", code, out.String(), errOut.String())
	}
	if code := run([]string{"validate", "--registry", reg, "--repo", "o/nothere"}, &out, &errOut, src, okFactory); code != 1 {
		t.Errorf("unknown --repo should fail, got %d", code)
	}
}

func TestRun_Build(t *testing.T) {
	reg, src := fixture(t)
	dist := filepath.Join(t.TempDir(), "dist")
	var out, errOut bytes.Buffer
	code := run([]string{"build", "--registry", reg, "--out", dist, "--base-url", "https://x.test/p"}, &out, &errOut, src, okFactory)
	if code != 2 {
		t.Errorf("a build with a skipped entry should exit 2, got %d (err=%q)", code, errOut.String())
	}
	if _, err := os.Stat(filepath.Join(dist, "index.json")); err != nil {
		t.Error("index.json should still be written")
	}
	if !strings.Contains(errOut.String(), "o/broken") {
		t.Errorf("skipped entry should be reported on stderr, got %q", errOut.String())
	}
	os.WriteFile(reg, []byte("plugins:\n  - repo: o/hello\n"), 0644)
	if code := run([]string{"build", "--registry", reg, "--out", dist}, &out, &errOut, src, okFactory); code != 0 {
		t.Errorf("clean build should exit 0, got %d", code)
	}
}

func TestRun_ImageFlagReachesValidatorFactory(t *testing.T) {
	reg, src := fixture(t)
	var out, errOut bytes.Buffer
	var gotImage string
	factory := func(image string) registry.Validator {
		gotImage = image
		return okValidator{}
	}
	code := run([]string{"validate", "--registry", reg, "--repo", "o/hello", "--image=custom:1"}, &out, &errOut, src, factory)
	if code != 0 {
		t.Fatalf("code=%d out=%q err=%q", code, out.String(), errOut.String())
	}
	if gotImage != "custom:1" {
		t.Errorf("--image=custom:1 should reach the validator factory, got %q", gotImage)
	}
}

func TestRun_FlagScoping(t *testing.T) {
	reg, src := fixture(t)
	var out, errOut bytes.Buffer
	if code := run([]string{"validate", "--registry", reg, "--out", "dist"}, &out, &errOut, src, okFactory); code != 2 || !strings.Contains(errOut.String(), "--out is not valid for validate") {
		t.Errorf("--out on validate: code=%d err=%q", code, errOut.String())
	}
	out.Reset()
	errOut.Reset()
	if code := run([]string{"validate", "--registry", reg, "--base-url", "https://x.test"}, &out, &errOut, src, okFactory); code != 2 || !strings.Contains(errOut.String(), "--base-url is not valid for validate") {
		t.Errorf("--base-url on validate: code=%d err=%q", code, errOut.String())
	}
	out.Reset()
	errOut.Reset()
	if code := run([]string{"build", "--registry", reg, "--repo", "o/hello"}, &out, &errOut, src, okFactory); code != 2 || !strings.Contains(errOut.String(), "--repo is not valid for build") {
		t.Errorf("--repo on build: code=%d err=%q", code, errOut.String())
	}
}

func TestRun_Usage(t *testing.T) {
	var out, errOut bytes.Buffer
	if code := run(nil, &out, &errOut, nil, nil); code != 2 || !strings.Contains(errOut.String(), "usage") {
		t.Errorf("code=%d err=%q", code, errOut.String())
	}
	if code := run([]string{"frobnicate"}, &out, &errOut, nil, nil); code != 2 {
		t.Errorf("unknown command: code=%d", code)
	}
}
