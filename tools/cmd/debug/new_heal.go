// Copyright 2025 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package main

import (
	"fmt"

	"github.com/spf13/cobra"
	"gitlab.com/accumulatenetwork/accumulate/tools/cmd/debug/new_heal"
)

var cmdNewHeal = &cobra.Command{
	Use:   "new-heal",
	Short: "New healing commands",
	Run:   newHeal,
}

func init() {
	cmd.AddCommand(cmdNewHeal)
	
	// Add subcommands
	cmdNewHeal.AddCommand(new_heal.StatusCmd())
}

func newHeal(_ *cobra.Command, _ []string) {
	fmt.Println("More to Come!")
}
