package registry

import (
	"os"
	"path/filepath"
	"testing"
)

func writeTemp(t *testing.T, name, content string) string {
	t.Helper()
	p := filepath.Join(t.TempDir(), name)
	if err := os.WriteFile(p, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}
	return p
}

func TestLoadRegistry(t *testing.T) {
	repos, err := LoadRegistry(writeTemp(t, "registry.yaml", "plugins:\n  - repo: goblogplatform/goblog-plugin-hello\n  - repo: someone/goblog-plugin-x\n"))
	if err != nil {
		t.Fatal(err)
	}
	if len(repos) != 2 || repos[0] != "goblogplatform/goblog-plugin-hello" || repos[1] != "someone/goblog-plugin-x" {
		t.Errorf("repos = %v", repos)
	}
}

func TestLoadRegistry_Errors(t *testing.T) {
	cases := map[string]string{
		"empty":     "plugins: []\n",
		"no key":    "repos:\n  - repo: a/b\n",
		"bad repo":  "plugins:\n  - repo: not-a-repo\n",
		"url repo":  "plugins:\n  - repo: https://github.com/a/b\n",
		"duplicate": "plugins:\n  - repo: a/b\n  - repo: a/b\n",
		"not yaml":  "plugins: [\n",
	}
	for name, src := range cases {
		if _, err := LoadRegistry(writeTemp(t, "registry.yaml", src)); err == nil {
			t.Errorf("%s: expected an error", name)
		}
	}
	if _, err := LoadRegistry(filepath.Join(t.TempDir(), "missing.yaml")); err == nil {
		t.Error("missing file: expected an error")
	}
}
