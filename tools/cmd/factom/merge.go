package main

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"
	"gitlab.com/accumulatenetwork/accumulate/internal/database"
	"gitlab.com/accumulatenetwork/accumulate/internal/database/snapshot"
	"gitlab.com/accumulatenetwork/accumulate/internal/errors"
)

var cmdMerge = &cobra.Command{
	Use:   "merge <output> <input directory>",
	Short: "Merge Factom snapshots",
	Args:  cobra.MinimumNArgs(2),
	Run:   merge,
}

func init() {
	cmd.AddCommand(cmdMerge)
	cmdMerge.Flags().StringVarP(&flagConvert.LogLevel, "log-level", "l", "error", "Set the logging level")
	cmdMerge.Flags().IntVarP(&flagConvert.StartFrom, "start-from", "s", 0, "Factom height to start from")
	cmdMerge.Flags().IntVar(&flagConvert.StopAt, "stop-at", 0, "Factom height to stop at (if zero, run to completion)")
}

func merge(_ *cobra.Command, args []string) {
	height := flagConvert.StartFrom

	ok := true
	onInterrupt(func() { ok = false })
	logger := newLogger()
	db, rm := tempBadger(logger)
	defer rm()
	defer db.Close()

	for ok {
		filename := filepath.Join(args[1], fmt.Sprintf("factom-%d.snapshot", height))

		input, err := os.Open(filename)
		if errors.Is(err, fs.ErrNotExist) {
			break
		}
		checkf(err, "read %s", filename)
		fmt.Printf("Importing %s\n", filename)
		check(snapshot.Restore(db, input, logger))

		height += 2000
		if flagConvert.StopAt > 0 && height >= flagConvert.StopAt {
			break
		}
	}
	fmt.Println("Interrupted")

	// Create a snapshot
	fmt.Printf("Saving as %s\n", args[0])
	f, err := os.Create(args[0])
	checkf(err, "open snapshot")
	defer f.Close()
	check(db.View(func(batch *database.Batch) error {
		_, err := snapshot.Collect(batch, f, func(*database.Account) (bool, error) { return true, nil })
		return err
	}))
}
