// Copyright 2022 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package pkg

import "fmt"

func noprint_print() {
	print() // want `Use a logger instead of printing`
}

func noprint_println() {
	println() // want `Use a logger instead of printing`
}

func noprint_fmt_Print() {
	fmt.Print() // want `Use a logger instead of printing`
}

func noprint_fmt_Printf() {
	fmt.Printf("") // want `Use a logger instead of printing`
}

func noprint_fmt_Println() {
	fmt.Println() // want `Use a logger instead of printing`
}
