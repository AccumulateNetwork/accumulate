// Copyright 2024 The Accumulate Authors
// 
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package main

import (
	"os"
	"os/signal"
	"syscall"

	"golang.org/x/sys/unix"
)

func onHUP(fn func()) {
	sigs := make(chan os.Signal, 1)
	signal.Notify(sigs, unix.SIGHUP)

	go func() {
		for range sigs {
			fn()
		}
	}()
}

func interrupt(pid int) {
	_ = syscall.Kill(pid, syscall.SIGINT)
}
