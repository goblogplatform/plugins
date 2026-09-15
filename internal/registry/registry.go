package registry

import (
	"fmt"
	"os"
	"regexp"

	"gopkg.in/yaml.v3"
)

var repoPattern = regexp.MustCompile(`^[A-Za-z0-9_.-]+/[A-Za-z0-9_.-]+$`)

// LoadRegistry reads registry.yaml and returns its repositories as
// owner/name strings in file order.
func LoadRegistry(path string) ([]string, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var doc struct {
		Plugins []struct {
			Repo string `yaml:"repo"`
		} `yaml:"plugins"`
	}
	if err := yaml.Unmarshal(b, &doc); err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	if len(doc.Plugins) == 0 {
		return nil, fmt.Errorf("%s: no plugins listed", path)
	}
	seen := map[string]bool{}
	repos := make([]string, 0, len(doc.Plugins))
	for i, p := range doc.Plugins {
		if !repoPattern.MatchString(p.Repo) {
			return nil, fmt.Errorf("%s: entry %d: repo %q must be owner/name", path, i+1, p.Repo)
		}
		if seen[p.Repo] {
			return nil, fmt.Errorf("%s: repo %q listed twice", path, p.Repo)
		}
		seen[p.Repo] = true
		repos = append(repos, p.Repo)
	}
	return repos, nil
}
