// Copyright 2023 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package run

import (
	"math"
	"unsafe"

	"golang.org/x/sys/windows"
)

var kernel32 = windows.MustLoadDLL("kernel32.dll")
var getDiskFreeSpaceExW = kernel32.MustFindProc("GetDiskFreeSpaceExW")

func diskUsage(path string) (float64, error) {
	pathPtr, err := windows.UTF16PtrFromString(path)
	if err != nil {
		return math.NaN(), err
	}

	var free, total int64
	ok, _, err := getDiskFreeSpaceExW.Call(
		uintptr(unsafe.Pointer(pathPtr)),
		uintptr(unsafe.Pointer(&free)),
		uintptr(unsafe.Pointer(&total)),
		0,
	)
	if ok != 1 {
		return math.NaN(), err
	}

	return float64(free) / float64(total), nil
}
