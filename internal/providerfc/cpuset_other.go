//go:build !linux

package providerfc

import "errors"

// pinCPUs is Linux-only, as Firecracker is.
func pinCPUs(pid int, cpuset string) error {
	if _, err := parseCPUSet(cpuset); err != nil {
		return err
	}
	return errors.New("CPU pinning needs Linux")
}
