//go:build windows

package providerch

import "os/exec"

// setSysProcAttr is a no-op on Windows; Cloud Hypervisor does not run on
// Windows.
func setSysProcAttr(_ *exec.Cmd) {}
