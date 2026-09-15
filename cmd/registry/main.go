// Command registry validates the plugin repositories listed in registry.yaml
// and builds the directory index published to GitHub Pages.
//
//	registry validate [--registry registry.yaml] [--image compscidr/goblog:vX.Y.Z] [--repo owner/name]
//	registry build    [--registry registry.yaml] [--image ...] [--out dist] [--base-url URL]
//
// GITHUB_TOKEN is used when set. Exit codes: 0 ok; 1 a validation failed or
// a fatal error; 2 either a usage error (unknown command, bad flags) or, for
// build, output was written but some entries were skipped.
package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"os"
	"sort"

	"github.com/goblogplatform/plugins/internal/registry"
)

const (
	defaultImage   = "compscidr/goblog:v0.2.7"
	defaultBaseURL = "https://goblogplatform.github.io/plugins"
)

func main() {
	src, err := registry.NewGitHubSource(os.Getenv("GITHUB_TOKEN"), "")
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	os.Exit(run(os.Args[1:], os.Stdout, os.Stderr, src, func(image string) registry.Validator {
		return registry.NewDockerValidator(image)
	}))
}

func usage(w io.Writer) {
	fmt.Fprintln(w, "usage: registry validate [--registry FILE] [--image IMAGE] [--repo owner/name]")
	fmt.Fprintln(w, "       registry build    [--registry FILE] [--image IMAGE] [--out DIR] [--base-url URL]")
}

func run(args []string, stdout, stderr io.Writer, src registry.Source, newValidator func(image string) registry.Validator) int {
	if len(args) == 0 {
		usage(stderr)
		return 2
	}
	fs := flag.NewFlagSet(args[0], flag.ContinueOnError)
	fs.SetOutput(stderr)
	regPath := fs.String("registry", "registry.yaml", "path to registry.yaml")
	image := fs.String("image", defaultImage, "goblog image used to load plugins")
	repo := fs.String("repo", "", "validate only this owner/name (must be listed)")
	out := fs.String("out", "dist", "build output directory")
	baseURL := fs.String("base-url", defaultBaseURL, "public URL the output is served from")

	switch args[0] {
	case "validate", "build":
	default:
		usage(stderr)
		return 2
	}
	if err := fs.Parse(args[1:]); err != nil {
		return 2
	}
	var disallowed map[string]bool
	switch args[0] {
	case "validate":
		disallowed = map[string]bool{"out": true, "base-url": true}
	case "build":
		disallowed = map[string]bool{"repo": true}
	}
	scopeErr := ""
	fs.Visit(func(f *flag.Flag) {
		if scopeErr == "" && disallowed[f.Name] {
			scopeErr = fmt.Sprintf("--%s is not valid for %s", f.Name, args[0])
		}
	})
	if scopeErr != "" {
		fmt.Fprintln(stderr, scopeErr)
		return 2
	}
	repos, err := registry.LoadRegistry(*regPath)
	if err != nil {
		fmt.Fprintln(stderr, err)
		return 1
	}
	val := newValidator(*image)
	ctx := context.Background()

	switch args[0] {
	case "validate":
		if *repo != "" {
			found := false
			for _, r := range repos {
				found = found || r == *repo
			}
			if !found {
				fmt.Fprintf(stderr, "%s is not listed in %s\n", *repo, *regPath)
				return 1
			}
			repos = []string{*repo}
		}
		failed := 0
		for _, r := range repos {
			v, err := registry.ValidateEntry(ctx, src, val, r)
			if err != nil {
				fmt.Fprintf(stderr, "%s: FAIL: %v\n", r, err)
				failed++
				continue
			}
			fmt.Fprintf(stdout, "%s: ok (%s %s)\n", r, v.Manifest.Name, v.Version)
		}
		if failed > 0 {
			return 1
		}
		return 0

	case "build":
		res, err := registry.Build(ctx, src, val, repos, *out, *baseURL)
		for _, r := range res.Built {
			fmt.Fprintf(stdout, "%s: built\n", r)
		}
		skipped := make([]string, 0, len(res.Skipped))
		for r := range res.Skipped {
			skipped = append(skipped, r)
		}
		sort.Strings(skipped)
		for _, r := range skipped {
			fmt.Fprintf(stderr, "%s: SKIPPED: %v\n", r, res.Skipped[r])
		}
		if err != nil {
			fmt.Fprintln(stderr, err)
			return 1
		}
		if len(skipped) > 0 {
			return 2
		}
		return 0
	}
	return 2
}
