//go:build unix

package hostagent

import "golang.org/x/sys/unix"

// diskMiB is the size and free space of the filesystem holding dir.
func diskMiB(dir string) (total, free int64) {
	var st unix.Statfs_t
	if err := unix.Statfs(dir, &st); err != nil {
		return 0, 0
	}
	bsize := uint64(st.Bsize) //nolint:unconvert // int64 on linux, uint32 on darwin
	return int64(uint64(st.Blocks) * bsize / (1 << 20)), int64(uint64(st.Bavail) * bsize / (1 << 20))
}

// kvmUsable reports whether this process can open /dev/kvm read-write.
func kvmUsable() bool {
	return unix.Access("/dev/kvm", unix.R_OK|unix.W_OK) == nil
}
