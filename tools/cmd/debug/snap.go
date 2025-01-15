// Copyright 2025 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package main

import "github.com/spf13/cobra"

var cmdSnap = &cobra.Command{
	Use:     "snapshot",
	Aliases: []string{"snap"},
	Short:   "Snapshot utilities",
}

func init() {
	cmd.AddCommand(cmdSnap)
}
