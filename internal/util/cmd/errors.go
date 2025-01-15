// Copyright 2025 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package cmdutil

import (
	"fmt"
	"os"

	"github.com/fatih/color"
	"golang.org/x/term"
)

func Fatalf(format string, args ...interface{}) {
	fmt.Fprintf(os.Stderr, "Error: "+format+"\n", args...)
	os.Exit(1)
}

func Check(err error) {
	if err != nil {
		Fatalf("%v", err)
	}
}

func Checkf(err error, format string, otherArgs ...interface{}) {
	if err != nil {
		Fatalf(format+": %v", append(otherArgs, err)...)
	}
}
func Warnf(format string, args ...interface{}) {
	format = "WARNING: " + format + "\n"
	if term.IsTerminal(int(os.Stderr.Fd())) {
		fmt.Fprint(os.Stderr, color.RedString(format, args...))
	} else {
		fmt.Fprintf(os.Stderr, format, args...)
	}
}
