//go:build !windows

package providerfirecracker

import (
	"os/exec"
	"syscall"
)

// setSysProcAttr starts the Firecracker process in a new session so it
// outlives the parent onctl process. Setsid is Unix-only.
func setSysProcAttr(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
}
