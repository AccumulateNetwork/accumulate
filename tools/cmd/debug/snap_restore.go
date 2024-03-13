// Copyright 2024 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package main

import (
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"time"

	"github.com/spf13/cobra"
	"gitlab.com/accumulatenetwork/accumulate/internal/core/execute"
	coredb "gitlab.com/accumulatenetwork/accumulate/internal/database"
	sv1 "gitlab.com/accumulatenetwork/accumulate/internal/database/snapshot"
	"gitlab.com/accumulatenetwork/accumulate/pkg/database"
	sv2 "gitlab.com/accumulatenetwork/accumulate/pkg/database/snapshot"
)

var cmdSnapRestore = &cobra.Command{
	Use:   "restore [snapshot] [database]",
	Short: "Restores a database from a snapshot",
	Args:  cobra.ExactArgs(2),
	Run:   restoreSnapshotCmd,
}

func init() {
	cmdSnap.AddCommand(cmdSnapRestore)
}

func restoreSnapshotCmd(cmd *cobra.Command, args []string) {
	// Verify that the target does not exist
	if _, err := os.Stat(args[1]); err == nil {
		fatalf("cannot restore snapshot into an existing database")
	} else if !errors.Is(err, fs.ErrNotExist) {
		check(err)
	}

	// Open the snapshot
	rd := openSnapshotFile(args[0])
	if c, ok := rd.(io.Closer); ok {
		defer c.Close()
	}
	ver, err := sv2.GetVersion(rd)
	check(err)

	// Rewind to the start
	_, err = rd.Seek(0, io.SeekStart)
	check(err)

	// Open the database
	db, err := coredb.OpenBadger(args[1], nil)
	check(err)
	db.SetObserver(execute.NewDatabaseObserver())

	// Timer for updating progress
	tick := time.NewTicker(time.Second / 2)
	defer tick.Stop()

	// Restore the snapshot
	switch ver {
	case sv1.Version1:
		check(sv1.Restore(db, rd, nil))
	case sv2.Version2:
		var metrics coredb.RestoreMetrics
		check(coredb.Restore(db, rd, &coredb.RestoreOptions{
			Metrics: &metrics,
			Predicate: func(r *sv2.RecordEntry, _ database.Value) (bool, error) {
				select {
				case <-tick.C:
				default:
					return true, nil
				}

				// The sole purpose of this function is to print progress
				h := r.Key.Hash()
				fmt.Printf("\033[A\r\033[KRestoring [%x] (%d) %v\n", h[:4], metrics.Records.Restoring, r.Key)

				// Restore everything
				return true, nil
			},
		}))
	default:
		fatalf("I don't know how to handle snapshot version %d", ver)
	}
}
