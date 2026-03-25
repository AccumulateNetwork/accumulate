//go:build dagbft

// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

// accumulated-dagbft is the Accumulate node binary using DAG-BFT consensus.
package main

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
)

func main() {
	root := &cobra.Command{
		Use:   "accumulated-dagbft",
		Short: "Accumulate node with DAG-BFT consensus",
		Long: `accumulated-dagbft runs an Accumulate node using DAG-BFT consensus
instead of CometBFT. This provides improved throughput and latency
through DAG-based transaction ordering.`,
	}

	root.AddCommand(
		cmdDevnet(),
		cmdRun(),
		cmdInit(),
		cmdVersion(),
	)

	if err := root.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

// cmdVersion returns the version command.
func cmdVersion() *cobra.Command {
	return &cobra.Command{
		Use:   "version",
		Short: "Print version information",
		Run: func(cmd *cobra.Command, args []string) {
			fmt.Println("accumulated-dagbft version 0.1.0")
		},
	}
}
