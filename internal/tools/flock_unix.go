//go:build !windows

package tools

import (
	"os"
	"syscall"
)

// Flock acquires an exclusive advisory lock on f.
func Flock(f *os.File) error {
	return syscall.Flock(int(f.Fd()), syscall.LOCK_EX)
}

// Funlock releases a lock acquired by Flock.
func Funlock(f *os.File) error {
	return syscall.Flock(int(f.Fd()), syscall.LOCK_UN)
}
