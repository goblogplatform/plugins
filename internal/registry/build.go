package registry

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

// IndexEntry is one element of index.json: the latest release of a plugin.
// Field names are the contract consumed by goblog's directory plugin and
// the admin installer; do not rename them.
type IndexEntry struct {
	Name             string   `json:"name"`
	DisplayName      string   `json:"display_name"`
	Description      string   `json:"description"`
	Version          string   `json:"version"`
	Author           string   `json:"author"`
	License          string   `json:"license"`
	SourceURL        string   `json:"source_url"`
	DownloadURL      string   `json:"download_url"`
	SHA256           string   `json:"sha256"`
	MinGoblogVersion string   `json:"min_goblog_version"`
	InstallType      string   `json:"install_type"`  // "wasm"
	Runtime          string   `json:"runtime"`       // "wasm"
	AllowedHosts     []string `json:"allowed_hosts"` // never null: [] when the plugin uses no network
	ReleasedAt       string   `json:"released_at"`
	DetailURL        string   `json:"detail_url"`
	Stars            int      `json:"stars"`
}

// ReleaseDoc is one release in a plugin's history.
type ReleaseDoc struct {
	Version    string `json:"version"`
	ReleasedAt string `json:"released_at"`
	NotesHTML  string `json:"notes_html"`
	URL        string `json:"url"`
}

// DetailDoc is plugins/<name>.json: the index entry plus rendered README,
// changelog and release history.
type DetailDoc struct {
	IndexEntry
	ReadmeHTML    string       `json:"readme_html"`
	ChangelogHTML string       `json:"changelog_html"`
	Releases      []ReleaseDoc `json:"releases"`
}

// BuildResult says which repositories made it into the index.
type BuildResult struct {
	Built   []string
	Skipped map[string]error
}

const indexHTML = `<!doctype html>
<meta charset="utf-8">
<title>goblog plugin registry</title>
<p>This is the machine-readable goblog plugin index. Browse the directory at
<a href="https://goblog.live/plugins">goblog.live/plugins</a>, or fetch
<a href="index.json">index.json</a>. To publish a plugin, see
<a href="https://github.com/goblogplatform/plugins">goblogplatform/plugins</a>.</p>
`

// Build validates every repository and writes index.json, plugins/<name>.json
// and index.html under outDir. A repository that fails validation is
// skipped and reported in the result so one broken release cannot take the
// directory down; the build fails outright only when nothing is valid.
func Build(ctx context.Context, src Source, val Validator, repos []string, outDir, baseURL string) (BuildResult, error) {
	res := BuildResult{Skipped: map[string]error{}}
	baseURL = strings.TrimSuffix(baseURL, "/")
	var index []IndexEntry
	var details []DetailDoc
	byName := map[string]string{} // plugin name → repo that claimed it

	for _, repo := range repos {
		v, err := ValidateEntry(ctx, src, val, repo)
		if err != nil {
			log.Printf("skip %s: %v", repo, err)
			res.Skipped[repo] = err
			continue
		}
		if prev, taken := byName[v.Manifest.Name]; taken {
			err := fmt.Errorf("%s: plugin name %q is already published by %s", repo, v.Manifest.Name, prev)
			log.Printf("skip %s: %v", repo, err)
			res.Skipped[repo] = err
			continue
		}
		d, err := buildDetail(ctx, src, v, baseURL)
		if err != nil {
			log.Printf("skip %s: %v", repo, err)
			res.Skipped[repo] = err
			continue
		}
		byName[v.Manifest.Name] = repo
		index = append(index, d.IndexEntry)
		details = append(details, d)
		res.Built = append(res.Built, repo)
	}
	if len(index) == 0 {
		return res, fmt.Errorf("no valid plugins; refusing to publish an empty index")
	}
	sort.Slice(index, func(i, j int) bool { return index[i].Name < index[j].Name })

	if err := os.MkdirAll(filepath.Join(outDir, "plugins"), 0755); err != nil {
		return res, err
	}
	if err := writeJSON(filepath.Join(outDir, "index.json"), index); err != nil {
		return res, err
	}
	for _, d := range details {
		if err := writeJSON(filepath.Join(outDir, "plugins", d.Name+".json"), d); err != nil {
			return res, err
		}
	}
	if err := os.WriteFile(filepath.Join(outDir, "index.html"), []byte(indexHTML), 0644); err != nil {
		return res, err
	}
	if err := os.WriteFile(filepath.Join(outDir, ".nojekyll"), nil, 0644); err != nil {
		return res, err
	}
	return res, nil
}

func buildDetail(ctx context.Context, src Source, v *Validated, baseURL string) (DetailDoc, error) {
	ownerRepo := v.Owner + "/" + v.Name
	entry := IndexEntry{
		Name:             v.Manifest.Name,
		DisplayName:      v.Manifest.DisplayName,
		Description:      v.Manifest.Description,
		Version:          v.Version,
		Author:           v.Manifest.Author,
		License:          v.Manifest.License,
		SourceURL:        "https://github.com/" + ownerRepo,
		DownloadURL:      v.Asset.DownloadURL,
		SHA256:           v.SHA256,
		MinGoblogVersion: v.Manifest.MinGoblogVersion,
		InstallType:      "wasm",
		Runtime:          "wasm",
		AllowedHosts:     v.Manifest.AllowedHosts,
		ReleasedAt:       v.Release.PublishedAt.UTC().Format(time.RFC3339),
		DetailURL:        fmt.Sprintf("%s/plugins/%s.json", baseURL, v.Manifest.Name),
	}

	// Stars only order the directory; a failed lookup must not drop an
	// otherwise valid plugin from the index.
	if stars, err := src.RepoStars(ctx, v.Owner, v.Name); err != nil {
		log.Printf("%s: stars unavailable, using 0: %v", ownerRepo, err)
	} else {
		entry.Stars = stars
	}

	readme, err := src.File(ctx, v.Owner, v.Name, v.Release.Tag, "README.md")
	if err != nil {
		return DetailDoc{}, fmt.Errorf("README.md: %w", err)
	}
	readmeHTML, err := src.RenderMarkdown(ctx, ownerRepo, string(readme))
	if err != nil {
		return DetailDoc{}, err
	}
	changelogHTML := ""
	if cl, err := src.File(ctx, v.Owner, v.Name, v.Release.Tag, "CHANGELOG.md"); err == nil {
		if changelogHTML, err = src.RenderMarkdown(ctx, ownerRepo, string(cl)); err != nil {
			return DetailDoc{}, err
		}
	} else if !isNotFound(err) {
		return DetailDoc{}, fmt.Errorf("CHANGELOG.md: %w", err)
	}

	releases := make([]ReleaseDoc, 0, len(v.Releases))
	for _, r := range v.Releases {
		notes, err := src.RenderMarkdown(ctx, ownerRepo, r.Body)
		if err != nil {
			return DetailDoc{}, err
		}
		releases = append(releases, ReleaseDoc{
			Version:    strings.TrimPrefix(r.Tag, "v"),
			ReleasedAt: r.PublishedAt.UTC().Format(time.RFC3339),
			NotesHTML:  notes,
			URL:        r.URL,
		})
	}
	return DetailDoc{IndexEntry: entry, ReadmeHTML: readmeHTML, ChangelogHTML: changelogHTML, Releases: releases}, nil
}

func writeJSON(path string, v any) error {
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	enc.SetIndent("", "  ")
	if err := enc.Encode(v); err != nil {
		return err
	}
	return os.WriteFile(path, buf.Bytes(), 0644)
}
