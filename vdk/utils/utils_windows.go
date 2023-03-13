// Copyright 2023 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package utils

import (
	"syscall"

	"golang.org/x/sys/windows"
)

var dllKernel = windows.NewLazySystemDLL("kernel32.dll")
var procGenerateConsoleCtrlEvent = dllKernel.NewProc("GenerateConsoleCtrlEvent")

func OnHUP(fn func()) {
	// Windows does not support SIGHUP
}

func Interrupt(pid int) {
	_, _, _ = procGenerateConsoleCtrlEvent.Call(syscall.CTRL_BREAK_EVENT, uintptr(pid))
}
