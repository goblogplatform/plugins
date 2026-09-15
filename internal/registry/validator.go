package registry

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

// Info is what `goblog validate-plugin` prints for a plugin file.
type Info struct {
	Name        string `json:"name"`
	DisplayName string `json:"display_name"`
	Version     string `json:"version"`
}

// Validator loads a plugin source file the way goblog would and reports its
// identity. The real one runs goblog's validate-plugin in Docker; tests use
// a fake.
type Validator interface {
	Validate(ctx context.Context, src []byte) (Info, error)
}

// GoblogEntrypoint is the goblog binary inside the release image, whose
// ENTRYPOINT is a shell command and therefore has to be overridden.
const GoblogEntrypoint = "/go/src/github.com/compscidr/goblog/goblog"

// defaultValidateTimeout bounds how long a single `docker run` is allowed to
// take before its container is killed and the plugin is rejected.
const defaultValidateTimeout = 120 * time.Second

// DockerValidator runs `goblog validate-plugin` inside the pinned goblog
// image with networking disabled: the file is interpreted, so it can run
// arbitrary Go, and this is the only sandbox the registry gives it.
type DockerValidator struct {
	Image string
	// Timeout bounds a single validation run. Defaults to 120s in
	// NewDockerValidator.
	Timeout time.Duration
}

func NewDockerValidator(image string) *DockerValidator {
	return &DockerValidator{Image: image, Timeout: defaultValidateTimeout}
}

func (d *DockerValidator) args(dir, name string) []string {
	return []string{"run", "--rm", "--network", "none", "--memory", "512m", "--pids-limit", "256",
		"--name", name, "-v", dir + ":/p:ro",
		"--entrypoint", GoblogEntrypoint, d.Image, "validate-plugin", "/p/plugin.go"}
}

// containerName generates a unique name for the container running one
// validation, so it can be targeted by `docker kill` when the context is
// cancelled or times out.
func containerName() (string, error) {
	b := make([]byte, 8)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return "goblog-validate-" + hex.EncodeToString(b), nil
}

func (d *DockerValidator) Validate(ctx context.Context, src []byte) (Info, error) {
	dir, err := os.MkdirTemp("", "goblog-plugin-")
	if err != nil {
		return Info{}, err
	}
	defer os.RemoveAll(dir)
	if err := os.WriteFile(filepath.Join(dir, "plugin.go"), src, 0644); err != nil {
		return Info{}, err
	}

	name, err := containerName()
	if err != nil {
		return Info{}, err
	}

	ctx, cancel := context.WithTimeout(ctx, d.Timeout)
	defer cancel()

	cmd := exec.CommandContext(ctx, "docker", d.args(dir, name)...)
	// Cancelling the context only terminates the "docker" CLI process, not
	// the container it started; kill the container by name so a timeout (or
	// caller cancellation) actually stops it instead of leaking it.
	cmd.Cancel = func() error { return exec.Command("docker", "kill", name).Run() }
	cmd.WaitDelay = 5 * time.Second
	var stdout, stderr bytes.Buffer
	cmd.Stdout, cmd.Stderr = &stdout, &stderr
	if err := cmd.Run(); err != nil {
		if ctx.Err() != nil {
			return Info{}, fmt.Errorf("validate-plugin: plugin timed out after %s: %s", d.Timeout, strings.TrimSpace(stderr.String()))
		}
		return Info{}, fmt.Errorf("validate-plugin failed: %s", strings.TrimSpace(stderr.String()+" "+err.Error()))
	}
	var info Info
	if err := json.Unmarshal(stdout.Bytes(), &info); err != nil {
		return Info{}, fmt.Errorf("validate-plugin printed %q: %w", stdout.String(), err)
	}
	return info, nil
}
