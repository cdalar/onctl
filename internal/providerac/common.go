package providerac

import (
	"bytes"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"strings"

	"github.com/cdalar/onctl/pkg/cloud"
	"github.com/spf13/viper"
)

const (
	defaultImage  = "docker.io/library/ubuntu:24.04"
	defaultBinary = "container"
)

// GetConfig reads the ac.* settings from viper, with the same fallbacks as
// the init template.
func GetConfig() cloud.ACConfig {
	image := viper.GetString("ac.image")
	if image == "" {
		image = defaultImage
	}
	vcpu := viper.GetInt64("ac.vcpuCount")
	if vcpu == 0 {
		vcpu = 1
	}
	mem := viper.GetInt64("ac.memSizeMib")
	if mem == 0 {
		mem = 2048
	}
	username := viper.GetString("ac.vm.username")
	if username == "" {
		username = "root"
	}
	binPath := viper.GetString("ac.binPath")
	if binPath == "" {
		binPath = defaultBinary
	}
	return cloud.ACConfig{
		Image:      image,
		VCPUCount:  vcpu,
		MemSizeMib: mem,
		Username:   username,
		BinPath:    binPath,
	}
}

// cliRunner runs Apple's `container` CLI as a subprocess.
type cliRunner struct {
	binPath string
}

// NewRunner returns a cloud.ACRunner that executes binPath.
func NewRunner(binPath string) cloud.ACRunner {
	return cliRunner{binPath: binPath}
}

// Stream runs the CLI with its output on the terminal, for commands that
// draw their own progress (image pull).
func (r cliRunner) Stream(args ...string) error {
	cmd := exec.Command(r.binPath, args...)
	cmd.Stdout = os.Stderr
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("container %s: %w", args[0], err)
	}
	return nil
}

func (r cliRunner) Run(args ...string) ([]byte, error) {
	cmd := exec.Command(r.binPath, args...)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		if errors.Is(err, exec.ErrNotFound) || errors.Is(err, fs.ErrNotExist) {
			return nil, fmt.Errorf("%w: the ac provider needs Apple's container CLI (https://github.com/apple/container) on macOS 26 or later", err)
		}
		msg := strings.TrimSpace(stderr.String())
		if msg == "" {
			msg = err.Error()
		}
		if strings.Contains(msg, "XPC connection error") {
			msg += " (is the container service running? try `container system start`)"
		}
		return nil, fmt.Errorf("container %s: %s", args[0], msg)
	}
	return stdout.Bytes(), nil
}
