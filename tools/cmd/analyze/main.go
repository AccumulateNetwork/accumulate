// Copyright 2025 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package main

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
	. "gitlab.com/accumulatenetwork/accumulate/internal/util/cmd"
	"gitlab.com/accumulatenetwork/accumulate/pkg/accumulate"
)

var (
	bootstrap = accumulate.BootstrapServers
)

var rootCmd = &cobra.Command{
	Use:   "analyze",
	Short: "Analysis utilities for Accumulate",
	Long:  "A collection of utilities for analyzing Accumulate databases, snapshots, and networks",
}

func init() {
	rootCmd.PersistentFlags().Var((*MultiaddrSliceFlag)(&bootstrap), "bootstrap", "Set the bootstrap servers")
	
	// Add commands directly to the root command
	// rootCmd.AddCommand(cmdAnalyzeDB) // Commented out - undefined reference
	rootCmd.AddCommand(cmdAnalyzeSnap)
	rootCmd.AddCommand(cmdAnalyzeSnapVersion)
	// rootCmd.AddCommand(cmdAnalyzePartition) // Commented out - undefined reference
	rootCmd.AddCommand(cmdAnalyzeSnapReport)
	// rootCmd.AddCommand(cmdAnalyzeSnapCombine) // Commented out - undefined reference
	rootCmd.AddCommand(sc_Cmd)
	rootCmd.AddCommand(cmdAnalyzeExtract) // Add the extract command
	rootCmd.AddCommand(InfoCommand()) // Add the info command
}

func main() {
	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}
