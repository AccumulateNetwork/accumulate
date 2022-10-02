// Copyright 2022 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

//go:build production
// +build production

package testing

import (
	"fmt"
	"os"

	"github.com/fatih/color"
)

func EnableDebugFeatures() {
	fmt.Fprintln(os.Stderr, color.RedString("Debugging features are not supported in production"))
	// os.Exit(1)
}
