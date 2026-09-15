package registry

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// FakeValidator answers by the sha256 of the source it is given.
type FakeValidator struct {
	Infos map[string]Info
	Err   error
}

func sum(b []byte) string {
	h := sha256.Sum256(b)
	return hex.EncodeToString(h[:])
}

func (f *FakeValidator) Validate(_ context.Context, src []byte) (Info, error) {
	if f.Err != nil {
		return Info{}, f.Err
	}
	if info, ok := f.Infos[sum(src)]; ok {
		return info, nil
	}
	return Info{}, errors.New("fake: does not load")
}

// TestDockerValidator_Real runs the actual goblog image; skipped unless
// docker is available and REGISTRY_DOCKER_TESTS=1 (it pulls ~100 MB).
func TestDockerValidator_Real(t *testing.T) {
	if _, err := exec.LookPath("docker"); err != nil || os.Getenv("REGISTRY_DOCKER_TESTS") == "" {
		t.Skip("set REGISTRY_DOCKER_TESTS=1 with docker available")
	}
	src := []byte(`package main
import "goblog/plugin"
type P struct{ plugin.BasePlugin }
func NewPlugin() plugin.Plugin { return &P{} }
func (P) Name() string { return "p" }
func (P) DisplayName() string { return "P" }
func (P) Version() string { return "1.2.3" }
`)
	v := NewDockerValidator("compscidr/goblog:v0.2.7")
	info, err := v.Validate(context.Background(), src)
	if err != nil {
		t.Fatal(err)
	}
	if info.Name != "p" || info.Version != "1.2.3" {
		t.Errorf("info = %+v", info)
	}
	if _, err := v.Validate(context.Background(), []byte("package main\nfunc NewPlugin() int { return 1 ")); err == nil {
		t.Error("broken source should fail")
	}
}

func TestDockerValidator_CommandShape(t *testing.T) {
	v := NewDockerValidator("compscidr/goblog:v0.2.7")
	args := v.args("/tmp/x", "goblog-validate-abc123")
	want := []string{"run", "--rm", "--network", "none", "--memory", "512m", "--pids-limit", "256",
		"--name", "goblog-validate-abc123", "-v", "/tmp/x:/p:ro",
		"--entrypoint", "/go/src/github.com/compscidr/goblog/goblog", "compscidr/goblog:v0.2.7",
		"validate-plugin", "/p/plugin.go"}
	if len(args) != len(want) {
		t.Fatalf("args = %v", args)
	}
	for i := range want {
		if args[i] != want[i] {
			t.Errorf("args[%d] = %q, want %q", i, args[i], want[i])
		}
	}
}

func TestNewDockerValidator_DefaultTimeout(t *testing.T) {
	v := NewDockerValidator("img")
	if v.Timeout != 120*time.Second {
		t.Errorf("default Timeout = %v, want 120s", v.Timeout)
	}
}

// TestDockerValidator_Timeout uses a fake "docker" on PATH that ignores
// "run" and hangs briefly, and answers "kill" by touching a marker file
// named after the container it was asked to kill, to check both that a
// short Timeout produces a "timed out" error (rather than hanging until the
// real 120s default) and that the kill path actually ran against the right
// container name, not just that the error message happens to say "timed
// out" (which it would even with cmd.Cancel left nil).
func TestDockerValidator_Timeout(t *testing.T) {
	dir := t.TempDir()
	markerDir := t.TempDir()
	script := "#!/bin/sh\ncase \"$1\" in\n  kill) touch \"$KILL_MARKER_DIR/$2\"; exit 0 ;;\n  *) sleep 0.3; exit 1 ;;\nesac\n"
	if err := os.WriteFile(filepath.Join(dir, "docker"), []byte(script), 0755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))
	t.Setenv("KILL_MARKER_DIR", markerDir)

	v := NewDockerValidator("img")
	v.Timeout = 50 * time.Millisecond
	_, err := v.Validate(context.Background(), []byte("package main\n"))
	if err == nil || !strings.Contains(err.Error(), "timed out") {
		t.Errorf("want a timed out error, got %v", err)
	}
	markers, err := filepath.Glob(filepath.Join(markerDir, "goblog-validate-*"))
	if err != nil {
		t.Fatal(err)
	}
	if len(markers) != 1 {
		t.Errorf("want docker kill to have run against exactly one goblog-validate-* container, got %v", markers)
	}
}
