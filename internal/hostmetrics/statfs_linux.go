//go:build linux

package hostmetrics

import "syscall"

// statfs aliases the Linux statfs struct so the shared collector can read
// filesystem usage without a platform-specific branch in its call sites.
type statfs = syscall.Statfs_t

func statfsCall(path string, out *statfs) error {
	return syscall.Statfs(path, out)
}
