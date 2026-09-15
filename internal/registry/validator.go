package registry

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
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

// DockerValidator runs `goblog validate-plugin` inside the pinned goblog
// image with networking disabled: the file is interpreted, so it can run
// arbitrary Go, and this is the only sandbox the registry gives it.
type DockerValidator struct {
	Image string
}

func NewDockerValidator(image string) *DockerValidator { return &DockerValidator{Image: image} }

func (d *DockerValidator) args(dir string) []string {
	return []string{"run", "--rm", "--network", "none", "-v", dir + ":/p:ro",
		"--entrypoint", GoblogEntrypoint, d.Image, "validate-plugin", "/p/plugin.go"}
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
	cmd := exec.CommandContext(ctx, "docker", d.args(dir)...)
	var stdout, stderr bytes.Buffer
	cmd.Stdout, cmd.Stderr = &stdout, &stderr
	if err := cmd.Run(); err != nil {
		return Info{}, fmt.Errorf("validate-plugin failed: %s", strings.TrimSpace(stderr.String()+" "+err.Error()))
	}
	var info Info
	if err := json.Unmarshal(stdout.Bytes(), &info); err != nil {
		return Info{}, fmt.Errorf("validate-plugin printed %q: %w", stdout.String(), err)
	}
	return info, nil
}
