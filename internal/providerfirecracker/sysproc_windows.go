//go:build windows

package providerfirecracker

import "os/exec"

// setSysProcAttr is a no-op on Windows; Firecracker does not run on Windows.
func setSysProcAttr(_ *exec.Cmd) {}
