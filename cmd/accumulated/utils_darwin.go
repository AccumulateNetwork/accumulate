package main

import (
	"os"
	"os/signal"

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
