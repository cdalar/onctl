//go:build windows

package providerfc

import "os/exec"

// setSysProcAttr is a no-op on Windows; Firecracker does not run on Windows.
func setSysProcAttr(_ *exec.Cmd) {}
