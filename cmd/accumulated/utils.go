// Copyright 2025 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package main

import (
	"github.com/spf13/cobra"
)

var DidError error

func printOutput(cmd *cobra.Command, out string, err error) {
	if err == nil {
		cmd.Println(out)
		return
	}

	DidError = err
	if out != "" {
		cmd.Println(out)
	}
	cmd.PrintErrf("Error: %v\n", err)
}
