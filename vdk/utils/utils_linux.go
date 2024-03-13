// Copyright 2024 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package utils

import (
	"os"
	"os/signal"
	"syscall"

	"golang.org/x/sys/unix"
)

func OnHUP(fn func()) {
	sigs := make(chan os.Signal, 1)
	signal.Notify(sigs, unix.SIGHUP)

	go func() {
		for range sigs {
			fn()
		}
	}()
}

func Interrupt(pid int) {
	_ = syscall.Kill(pid, syscall.SIGINT)
}
