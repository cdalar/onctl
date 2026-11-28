//go:build !unix

package hostagent

// diskMiB is unknown off Unix: a box host runs Linux.
func diskMiB(string) (total, free int64) { return 0, 0 }

// kvmUsable is false off Unix: KVM is Linux's.
func kvmUsable() bool { return false }
