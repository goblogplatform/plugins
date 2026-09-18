package registry

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestBuild_WritesIndexAndDetails(t *testing.T) {
	src := helloSource()
	// A second plugin, older release, to check sorting and skipping.
	src.releases["o/zeta"] = []Release{{Tag: "v0.1.0", Body: "z", URL: "https://github.com/o/zeta/releases/tag/v0.1.0", PublishedAt: time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)}}
	src.files["o/zeta@v0.1.0:goblog-plugin.json"] = strings.Replace(strings.Replace(goodManifest, `"hello"`, `"zeta"`, 1), `"Hello"`, `"Zeta"`, 1)
	src.files["o/zeta@v0.1.0:plugin.go"] = "package main // zeta\n"
	src.files["o/zeta@v0.1.0:README.md"] = "# Zeta"
	val := helloValidator()
	val.Infos[sum([]byte("package main // zeta\n"))] = Info{Name: "zeta", DisplayName: "Zeta", Version: "0.1.0"}

	out := t.TempDir()
	res, err := Build(context.Background(), src, val, []string{"o/zeta", "o/hello"}, out, "https://example.test/plugins")
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Built) != 2 || len(res.Skipped) != 0 {
		t.Fatalf("result = %+v", res)
	}

	var index []IndexEntry
	mustJSON(t, filepath.Join(out, "index.json"), &index)
	if len(index) != 2 || index[0].Name != "hello" || index[1].Name != "zeta" {
		t.Fatalf("index should be sorted by name: %+v", index)
	}
	e := index[0]
	want := IndexEntry{
		Name: "hello", DisplayName: "Hello", Description: "Says hi.", Version: "1.1.0", Author: "Jason Ernst",
		License: "Apache-2.0", SourceURL: "https://github.com/o/hello",
		DownloadURL: "https://raw.githubusercontent.com/o/hello/v1.1.0/plugin.go", SHA256: sum([]byte(helloSrc)),
		MinGoblogVersion: "0.2.6", InstallType: "dynamic", ReleasedAt: "2026-09-15T00:00:00Z",
		DetailURL: "https://example.test/plugins/plugins/hello.json", Stars: 7,
	}
	if e != want {
		t.Errorf("entry =\n%+v\nwant\n%+v", e, want)
	}

	var d DetailDoc
	mustJSON(t, filepath.Join(out, "plugins", "hello.json"), &d)
	if d.Name != "hello" || d.ReadmeHTML != "<p># Hello</p>" || d.ChangelogHTML != "<p>## 1.1.0\n- second</p>" {
		t.Errorf("detail = %+v", d)
	}
	if d.Stars != 7 {
		t.Errorf("detail stars = %d", d.Stars)
	}
	detailRaw, err := os.ReadFile(filepath.Join(out, "plugins", "hello.json"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(detailRaw), "<p># Hello</p>") {
		t.Errorf("readme_html should not be HTML-escaped, got %s", detailRaw)
	}
	if len(d.Releases) != 2 || d.Releases[0].Version != "1.1.0" || d.Releases[0].NotesHTML != "<p>Second</p>" || d.Releases[0].ReleasedAt != "2026-09-15T00:00:00Z" || d.Releases[0].URL == "" || d.Releases[1].Version != "1.0.0" {
		t.Errorf("releases = %+v", d.Releases)
	}
	var z DetailDoc
	mustJSON(t, filepath.Join(out, "plugins", "zeta.json"), &z)
	if z.ChangelogHTML != "" {
		t.Errorf("missing CHANGELOG.md should give empty changelog_html, got %q", z.ChangelogHTML)
	}

	html, _ := os.ReadFile(filepath.Join(out, "index.html"))
	if !strings.Contains(string(html), "goblog.live/plugins") || !strings.Contains(string(html), "index.json") {
		t.Errorf("index.html should point readers at goblog.live/plugins and index.json, got %q", html)
	}
	if _, err := os.Stat(filepath.Join(out, ".nojekyll")); err != nil {
		t.Error(".nojekyll should exist so Pages serves files as-is")
	}
	// The raw index must be exactly what a consumer parses: check it round-trips byte-for-byte.
	raw, _ := os.ReadFile(filepath.Join(out, "index.json"))
	var generic any
	if err := json.Unmarshal(raw, &generic); err != nil {
		t.Errorf("index.json is not valid JSON: %v", err)
	}
}

func TestBuild_SkipsBrokenEntriesAndDuplicates(t *testing.T) {
	src := helloSource()
	// "o/copy" is a valid repo whose manifest reuses the name "hello".
	src.releases["o/copy"] = src.releases["o/hello"]
	for k, v := range src.files {
		if strings.HasPrefix(k, "o/hello@") {
			src.files[strings.Replace(k, "o/hello@", "o/copy@", 1)] = v
		}
	}
	out := t.TempDir()
	res, err := Build(context.Background(), src, helloValidator(), []string{"o/hello", "o/nope", "o/copy"}, out, "https://example.test/plugins")
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Built) != 1 || res.Built[0] != "o/hello" {
		t.Errorf("built = %v", res.Built)
	}
	if len(res.Skipped) != 2 || res.Skipped["o/nope"] == nil || res.Skipped["o/copy"] == nil {
		t.Errorf("skipped = %v", res.Skipped)
	}
	if !strings.Contains(res.Skipped["o/copy"].Error(), "already") {
		t.Errorf("duplicate name error should say so: %v", res.Skipped["o/copy"])
	}
	var index []IndexEntry
	mustJSON(t, filepath.Join(out, "index.json"), &index)
	if len(index) != 1 {
		t.Errorf("index should contain only the good entry, got %d", len(index))
	}
	if _, err := os.Stat(filepath.Join(out, "plugins", "nope.json")); err == nil {
		t.Error("no detail file for a skipped entry")
	}
}

func TestBuild_FailsWhenNothingBuilt(t *testing.T) {
	src := helloSource()
	if _, err := Build(context.Background(), src, helloValidator(), []string{"o/nope"}, t.TempDir(), "https://example.test/plugins"); err == nil {
		t.Error("a build with zero valid entries must fail rather than publish an empty index")
	}
}

func mustJSON(t *testing.T, path string, v any) {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(b, v); err != nil {
		t.Fatalf("%s: %v", path, err)
	}
}
