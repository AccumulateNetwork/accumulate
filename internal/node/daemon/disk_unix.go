//go:build linux || darwin
// +build linux darwin

package daemon

import (
	"math"

	"golang.org/x/sys/unix"
)

func diskUsage(path string) (float64, error) {
	var stat unix.Statfs_t
	err := unix.Statfs(path, &stat)
	if err != nil {
		return math.NaN(), err
	}

	return float64(stat.Bavail) / float64(stat.Blocks), nil
}
