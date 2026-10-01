//go:build linux

package ops

import "syscall"

// diskUsage reads how full the file system holding path is. It needs no
// write access, so it works under the service's read-only file system.
func diskUsage(path string) (Disk, error) {
	var st syscall.Statfs_t
	if err := syscall.Statfs(path, &st); err != nil {
		return Disk{}, err
	}
	bsize := uint64(st.Bsize) //nolint:gosec // block size is never negative
	return Disk{
		Total: st.Blocks * bsize,
		Free:  st.Bavail * bsize,
		Used:  (st.Blocks - st.Bfree) * bsize,
	}, nil
}
