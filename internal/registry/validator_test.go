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

// FakeValidator answers by the sha256 of the module bytes it is given.
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

// realImage is the goblog image the real Docker test runs; keep it equal to
// the default in cmd/registry and the workflows.
const realImage = "compscidr/goblog:v0.2.9"

// echoWasmPath is goblog's committed echo fixture (identity echo/Echo/1.2.3),
// found when this registry is checked out next to goblog.
const echoWasmPath = "../../../goblog/plugin/wasm/testdata/echo.wasm"

// TestDockerValidator_Real runs the actual goblog image against goblog's
// echo.wasm fixture; skipped unless docker is available,
// REGISTRY_DOCKER_TESTS=1 (it pulls ~100 MB) and the fixture is present.
func TestDockerValidator_Real(t *testing.T) {
	if _, err := exec.LookPath("docker"); err != nil || os.Getenv("REGISTRY_DOCKER_TESTS") == "" {
		t.Skip("set REGISTRY_DOCKER_TESTS=1 with docker available")
	}
	module, err := os.ReadFile(echoWasmPath)
	if err != nil {
		t.Skipf("goblog's echo.wasm fixture not found at %s: %v", echoWasmPath, err)
	}
	v := NewDockerValidator(realImage)
	info, err := v.Validate(context.Background(), module)
	if err != nil {
		t.Fatal(err)
	}
	if info.Name != "echo" || info.DisplayName != "Echo" || info.Version != "1.2.3" || info.Runtime != "wasm" {
		t.Errorf("info = %+v", info)
	}
	if _, err := v.Validate(context.Background(), []byte("\x00asm not a module")); err == nil {
		t.Error("a broken module should fail")
	}
}

func TestDockerValidator_CommandShape(t *testing.T) {
	v := NewDockerValidator(realImage)
	args := v.args("/tmp/x", "goblog-validate-abc123")
	want := []string{"run", "--rm", "--network", "none", "--memory", "512m", "--pids-limit", "256",
		"--name", "goblog-validate-abc123", "-v", "/tmp/x:/p:ro",
		"--entrypoint", "/go/src/github.com/compscidr/goblog/goblog", "compscidr/goblog:v0.2.9",
		"validate-plugin", "/p/plugin.wasm"}
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
	_, err := v.Validate(context.Background(), []byte("\x00asm"))
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

// TestDockerValidator_WritesPluginWasm uses a fake "docker" that reads the
// mounted directory out of the -v argument and prints the sha256 of the
// plugin.wasm it finds there as the identity's name, so the test can check
// that the module bytes reach the container under the name validate-plugin
// is told to load.
func TestDockerValidator_WritesPluginWasm(t *testing.T) {
	dir := t.TempDir()
	script := `#!/bin/sh
prev=""
for a in "$@"; do
  if [ "$prev" = "-v" ]; then mount="${a%%:*}"; fi
  prev="$a"
done
last=""
for a in "$@"; do last="$a"; done
[ "$last" = "/p/plugin.wasm" ] || { echo "unexpected file $last" >&2; exit 1; }
sum=$(sha256sum "$mount/plugin.wasm" | cut -d' ' -f1)
printf '{"name":"%s","display_name":"X","version":"0.0.1","runtime":"wasm"}\n' "$sum"
`
	if err := os.WriteFile(filepath.Join(dir, "docker"), []byte(script), 0755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))

	module := []byte("\x00asm module bytes")
	info, err := NewDockerValidator("img").Validate(context.Background(), module)
	if err != nil {
		t.Fatal(err)
	}
	if info.Name != sum(module) || info.Runtime != "wasm" {
		t.Errorf("info = %+v, want name %s", info, sum(module))
	}
}
