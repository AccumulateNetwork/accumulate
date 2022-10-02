// Copyright 2022 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package main

import (
	"fmt"
	"os"
	"path/filepath"
	"text/tabwriter"

	"github.com/spf13/cobra"
	"gitlab.com/accumulatenetwork/accumulate/internal/core"
	"gitlab.com/accumulatenetwork/accumulate/internal/database/snapshot"
)

var listCmd = &cobra.Command{
	Use:   "list <directory>",
	Short: "List snapshots",
	Args:  cobra.ExactArgs(1),
	Run:   listSnapshots,
}

func init() { cmd.AddCommand(listCmd) }

func listSnapshots(_ *cobra.Command, args []string) {
	snapDir := args[0]
	entries, err := os.ReadDir(snapDir)
	checkf(err, "read directory")

	wr := tabwriter.NewWriter(os.Stdout, 3, 4, 2, ' ', 0)
	defer wr.Flush()

	fmt.Fprint(wr, "HEIGHT\tHASH\tFILE\n")
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		if !core.SnapshotMajorRegexp.MatchString(entry.Name()) {
			continue
		}

		filename := filepath.Join(snapDir, entry.Name())
		f, err := os.Open(filename)
		checkf(err, "open snapshot %s", entry.Name())
		defer f.Close()

		header, _, err := snapshot.Open(f)
		checkf(err, "oepn snapshot %s", entry.Name())

		fmt.Fprintf(wr, "%d\t%x\t%s\n", header.Height, header.RootHash, entry.Name())
	}
}
