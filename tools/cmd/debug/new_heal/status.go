// Copyright 2025 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package new_heal

import (
	"fmt"

	"github.com/spf13/cobra"
)

// StatusCmd returns the status subcommand
func StatusCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "status",
		Short: "Show healing status",
		Run:   statusCmd,
	}
	return cmd
}

func statusCmd(_ *cobra.Command, _ []string) {
	fmt.Println("status")
}
