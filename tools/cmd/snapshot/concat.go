package main

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"
	"gitlab.com/accumulatenetwork/accumulate/internal/database/snapshot"
)

var concatCmd = &cobra.Command{
	Use:   "concat <output> <inputs>",
	Short: "Concatenate multiple snapshots into one",
	Args:  cobra.MinimumNArgs(2),
	Run:   concatSnapshots,
}

func init() { cmd.AddCommand(concatCmd) }

func concatSnapshots(_ *cobra.Command, args []string) {
	wf, err := os.Create(args[0])
	checkf(err, "output file")
	w := snapshot.NewWriter(wf)

	for i, filename := range args[1:] {
		rf, err := os.Open(filename)
		checkf(err, "input file %d", i)
		defer rf.Close()

		r := snapshot.NewReader(rf)
		for {
			s, err := r.Next()
			if errors.Is(err, io.EOF) {
				break
			}
			checkf(err, "next section")
			fmt.Printf("Copying %s section of %s\n", s.Type(), filepath.Base(filename))

			sr, err := s.Open()
			checkf(err, "read section")

			sw, err := w.Open(s.Type())
			checkf(err, "write section")

			_, err = io.Copy(sw, sr)
			checkf(err, "copy section")

			check(sw.Close())
		}
	}
	check(wf.Close())
}
