package registry

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"time"

	"github.com/google/go-github/v92/github"
)

// ErrNotFound is returned by Source.File when the ref or path does not exist.
var ErrNotFound = errors.New("not found")

// Release is one GitHub release of a plugin repository.
type Release struct {
	Tag         string
	Name        string
	Body        string // release notes, markdown
	URL         string
	PublishedAt time.Time
	Draft       bool
	Prerelease  bool
}

// Source is what the registry needs from GitHub. It is an interface so the
// validator and builder are tested against an httptest fake.
type Source interface {
	// Releases lists all releases, newest first as GitHub returns them,
	// including drafts and pre-releases (callers filter).
	Releases(ctx context.Context, owner, repo string) ([]Release, error)
	// File returns the contents of path at ref; ErrNotFound when absent.
	File(ctx context.Context, owner, repo, ref, path string) ([]byte, error)
	// RenderMarkdown renders GitHub-flavoured markdown to sanitized HTML in
	// the context of ownerRepo (so #123 and relative links resolve).
	RenderMarkdown(ctx context.Context, ownerRepo, markdown string) (string, error)
}

// GitHubSource implements Source with the GitHub REST API.
type GitHubSource struct {
	client *github.Client
}

// NewGitHubSource returns a Source for api.github.com (baseURL "") or a
// test server. token may be empty for unauthenticated access.
func NewGitHubSource(token, baseURL string) (*GitHubSource, error) {
	opts := []github.ClientOptionsFunc{
		github.WithHTTPClient(&http.Client{Timeout: 30 * time.Second}),
		github.WithUserAgent("goblog-plugin-registry"),
	}
	if token != "" {
		opts = append(opts, github.WithAuthToken(token))
	}
	if baseURL != "" {
		opts = append(opts, github.WithURLs(&baseURL, &baseURL))
	}
	c, err := github.NewClient(opts...)
	if err != nil {
		return nil, err
	}
	return &GitHubSource{client: c}, nil
}

func (g *GitHubSource) Releases(ctx context.Context, owner, repo string) ([]Release, error) {
	var out []Release
	opts := &github.ListOptions{PerPage: 100}
	for {
		page, resp, err := g.client.Repositories.ListReleases(ctx, owner, repo, opts)
		if err != nil {
			return nil, fmt.Errorf("list releases for %s/%s: %w", owner, repo, err)
		}
		for _, r := range page {
			rel := Release{Tag: r.TagName, URL: r.HTMLURL, Draft: r.Draft, Prerelease: r.Prerelease}
			if r.Name != nil {
				rel.Name = *r.Name
			}
			if r.Body != nil {
				rel.Body = *r.Body
			}
			if r.PublishedAt != nil {
				rel.PublishedAt = r.PublishedAt.Time
			}
			out = append(out, rel)
		}
		if resp.NextPage == 0 {
			break
		}
		opts.Page = resp.NextPage
	}
	return out, nil
}

func (g *GitHubSource) File(ctx context.Context, owner, repo, ref, path string) ([]byte, error) {
	fc, _, resp, err := g.client.Repositories.GetContents(ctx, owner, repo, path, &github.RepositoryContentGetOptions{Ref: ref})
	if err != nil {
		if resp != nil && resp.StatusCode == http.StatusNotFound {
			return nil, fmt.Errorf("%s/%s@%s:%s: %w", owner, repo, ref, path, ErrNotFound)
		}
		return nil, fmt.Errorf("get %s/%s@%s:%s: %w", owner, repo, ref, path, err)
	}
	if fc == nil {
		return nil, fmt.Errorf("%s/%s@%s:%s is not a file", owner, repo, ref, path)
	}
	s, err := fc.GetContent()
	if err != nil {
		return nil, fmt.Errorf("decode %s/%s@%s:%s: %w", owner, repo, ref, path, err)
	}
	return []byte(s), nil
}

func (g *GitHubSource) RenderMarkdown(ctx context.Context, ownerRepo, markdown string) (string, error) {
	if markdown == "" {
		return "", nil
	}
	html, _, err := g.client.Markdown.Render(ctx, markdown, &github.MarkdownOptions{Mode: "gfm", Context: ownerRepo})
	if err != nil {
		return "", fmt.Errorf("render markdown for %s: %w", ownerRepo, err)
	}
	return html, nil
}
