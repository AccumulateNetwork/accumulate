// Copyright 2025 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package main

import (
	"github.com/spf13/cobra"
)

var cmdDb = &cobra.Command{
	Use:     "database",
	Aliases: []string{"db"},
	Short:   "Database utilities",
}

func init() {
	cmd.AddCommand(cmdDb)
}
