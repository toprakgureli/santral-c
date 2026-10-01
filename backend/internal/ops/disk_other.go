//go:build !linux

package ops

func diskUsage(string) (Disk, error) {
	return Disk{}, errNoDiskCheck
}
