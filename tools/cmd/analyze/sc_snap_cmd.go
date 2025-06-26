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
)

// Command for the snapshot analysis functionality
var cmdAnalyzeSnapSections = &cobra.Command{
	Use:   "sections [snapshot-file]",
	Short: "Analyze snapshot sections and their contents",
	Long: `Analyze a snapshot file by parsing each section and displaying detailed information.

For the header section (type 1), prints detailed header data.
For record sections (type 7), counts records by type (account, transaction, etc.).
For index sections (type 8), prints the total number of index entries.
For BPT sections (types 9 and 11), counts the total number of entries.`,
	Args: cobra.ExactArgs(1), // Requires exactly one argument (the snapshot file path)
	RunE: func(cmd *cobra.Command, args []string) error {
		// Get the snapshot file path from arguments
		snapshotPath := args[0]

		// Open the snapshot file
		file, err := os.Open(snapshotPath)
		if err != nil {
			return fmt.Errorf("failed to open snapshot file: %w", err)
		}
		defer file.Close()

		// Create a state object with the file
		state := &sc_State{
			InputFiles: []*os.File{file},
		}

		// Call the sectionScan function
		return sectionScan(state)
	},
}

func init() {
	// Add the sections command to the snap command
	cmdAnalyzeSnap.AddCommand(cmdAnalyzeSnapSections)
}
