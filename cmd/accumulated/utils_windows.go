package main

import (
	"os"
	"os/signal"

	"golang.org/x/sys/unix"
)

func onHUP(fn func()) {
	// Windows does not support SIGHUP
}
