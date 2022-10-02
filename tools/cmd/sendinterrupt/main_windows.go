// Copyright 2022 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package main

import (
	"log"
	"os"
	"strconv"
	"syscall"

	"golang.org/x/sys/windows"
)

func main() {
	log.SetFlags(log.LstdFlags | log.Lshortfile)
	dll, err := windows.LoadDLL("kernel32.dll")
	if err != nil {
		log.Fatal(err)
	}
	defer dll.Release()

	pid, err := strconv.ParseInt(os.Args[1], 10, 32)
	if err != nil {
		log.Fatal(err)
	}

	f, err := dll.FindProc("AttachConsole")
	if err != nil {
		log.Fatal(err)
	}
	r1, _, err := f.Call(uintptr(pid))
	if r1 == 0 && err != syscall.ERROR_ACCESS_DENIED {
		log.Fatal(err)
	}

	f, err = dll.FindProc("SetConsoleCtrlHandler")
	if err != nil {
		log.Fatal(err)
	}
	r1, _, err = f.Call(0, 1)
	if r1 == 0 {
		log.Fatal(err)
	}
	f, err = dll.FindProc("GenerateConsoleCtrlEvent")
	if err != nil {
		log.Fatal(err)
	}
	r1, _, err = f.Call(windows.CTRL_BREAK_EVENT, uintptr(pid))
	if r1 == 0 {
		log.Fatal(err)
	}
}
