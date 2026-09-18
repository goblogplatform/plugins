package registry

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// fakeGitHub serves the three REST endpoints GitHubSource uses.
func fakeGitHub(t *testing.T) *httptest.Server {
	t.Helper()
	mux := http.NewServeMux()
	mux.HandleFunc("GET /repos/o/r/releases", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`[
		  {"tag_name":"v1.1.0","name":"v1.1.0","body":"Second","draft":false,"prerelease":false,"published_at":"2026-09-15T00:00:00Z","html_url":"https://github.com/o/r/releases/tag/v1.1.0"},
		  {"tag_name":"v1.2.0-rc1","name":"rc","body":"","draft":false,"prerelease":true,"published_at":"2026-09-16T00:00:00Z","html_url":"https://github.com/o/r/releases/tag/v1.2.0-rc1"},
		  {"tag_name":"v1.0.0","name":"v1.0.0","body":"First","draft":false,"prerelease":false,"published_at":"2026-09-14T00:00:00Z","html_url":"https://github.com/o/r/releases/tag/v1.0.0"}
		]`))
	})
	mux.HandleFunc("GET /repos/o/r", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"full_name":"o/r","stargazers_count":42}`))
	})
	mux.HandleFunc("GET /repos/o/r/contents/plugin.go", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("ref") != "v1.1.0" {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"type":"file","encoding":"base64","content":"` + base64.StdEncoding.EncodeToString([]byte("package main\n")) + `"}`))
	})
	mux.HandleFunc("POST /markdown", func(w http.ResponseWriter, r *http.Request) {
		var body struct{ Text, Mode, Context string }
		if err := jsonDecode(r, &body); err != nil {
			http.Error(w, err.Error(), 400)
			return
		}
		if body.Mode != "gfm" || body.Context != "o/r" {
			http.Error(w, "expected gfm mode with repo context", 400)
			return
		}
		w.Header().Set("Content-Type", "text/html")
		w.Write([]byte("<p>" + body.Text + "</p>"))
	})
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) { http.NotFound(w, r) })
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return srv
}

func TestGitHubSource(t *testing.T) {
	srv := fakeGitHub(t)
	src, err := NewGitHubSource("", srv.URL+"/")
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()

	rels, err := src.Releases(ctx, "o", "r")
	if err != nil {
		t.Fatal(err)
	}
	if len(rels) != 3 || rels[0].Tag != "v1.1.0" || rels[1].Prerelease != true || rels[2].Body != "First" || rels[0].URL == "" || rels[0].PublishedAt.IsZero() {
		t.Errorf("releases = %+v", rels)
	}

	b, err := src.File(ctx, "o", "r", "v1.1.0", "plugin.go")
	if err != nil || string(b) != "package main\n" {
		t.Errorf("File: %q %v", b, err)
	}
	if _, err := src.File(ctx, "o", "r", "v9.9.9", "plugin.go"); !errors.Is(err, ErrNotFound) {
		t.Errorf("missing ref should be ErrNotFound, got %v", err)
	}
	if _, err := src.File(ctx, "o", "r", "v1.1.0", "CHANGELOG.md"); !errors.Is(err, ErrNotFound) {
		t.Errorf("missing file should be ErrNotFound, got %v", err)
	}

	html, err := src.RenderMarkdown(ctx, "o/r", "hi")
	if err != nil || !strings.Contains(html, "<p>hi</p>") {
		t.Errorf("RenderMarkdown: %q %v", html, err)
	}
	if html, err := src.RenderMarkdown(ctx, "o/r", ""); err != nil || html != "" {
		t.Errorf("empty markdown should render to empty string without a request, got %q %v", html, err)
	}

	stars, err := src.RepoInfo(ctx, "o", "r")
	if err != nil || stars != 42 {
		t.Errorf("RepoInfo = %d, %v", stars, err)
	}
	if _, err := src.RepoInfo(ctx, "o", "missing"); err == nil {
		t.Error("RepoInfo on an unknown repo should fail")
	}
}

func jsonDecode(r *http.Request, v any) error { return json.NewDecoder(r.Body).Decode(v) }
