//go:build windows

package tools

import "os"

// Flock and Funlock are no-ops on Windows: the Firecracker provider these
// locks serialize requires KVM and never runs there, so there's no
// concurrent access to guard against. They exist only so callers compile.
func Flock(f *os.File) error   { return nil }
func Funlock(f *os.File) error { return nil }
