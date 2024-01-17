// Copyright 2023 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

//go:build linux || darwin
// +build linux darwin

package run

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
