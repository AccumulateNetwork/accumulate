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
