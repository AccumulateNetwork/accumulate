// Copyright 2025 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

// Package main implements snapshot extraction tools.
// IMPORTANT: This implementation uses a streaming architecture to process snapshots
// efficiently without loading the entire database into memory. The goal is to process
// a 2GB snapshot using less than 5GB of memory (compared to previous implementations
// that required 40GB for a 2GB snapshot).
package main

import (
	"fmt"
	"os"
	"strings"

	"github.com/spf13/cobra"
)

// cmdAnalyze is the root command for analysis tools
var cmdAnalyze = &cobra.Command{
	Use:   "analyze",
	Short: "Analysis tools for Accumulate",
}

// cmdAnalyzeExtract is the cobra command for extracting data from a snapshot
var cmdAnalyzeExtract = &cobra.Command{
	Use:   "extract <network.json> <snapshot>",
	Short: "Extract data from a unified snapshot using network configuration",
	Run: func(cmd *cobra.Command, args []string) {
		if len(args) != 2 {
			cmd.Usage()
			os.Exit(1)
		}
		networkFile := args[0]
		snapshotFile := args[1]

		// Create a new ExtractState
		state := NewExtractState()
		state.SnapshotFile = snapshotFile
		state.NetworkFile = networkFile

		// Get validator keys from command line
		validatorKeys, _ := cmd.Flags().GetStringSlice("validator-keys")
		if len(validatorKeys) > 0 {
			// Trim whitespace from each key
			for i, key := range validatorKeys {
				validatorKeys[i] = strings.TrimSpace(key)
			}
			state.ValidatorKeys = validatorKeys
		}

		// Run the extraction
		err := state.Run()
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
			os.Exit(1)
		}
	},
	Args: cobra.ExactArgs(2),
}

// init adds flags to the command and registers commands
func init() {
	// Add flags to the extract command
	cmdAnalyzeExtract.Flags().StringSlice("validator-keys", nil, "Comma-separated list of validator public keys (hex encoded)")

	// Register commands
	cmdAnalyze.AddCommand(cmdAnalyzeExtract)
	cmdAnalyze.AddCommand(InfoCommand())
}
