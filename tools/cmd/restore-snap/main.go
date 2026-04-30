// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

// restore-snap takes an Accumulate snapshot file and restores it
// into a fresh Badger DB at the given output path. Used to
// reproduce a state-synced node's DB content for diffing against a
// real validator's DB.
package main

import (
	"fmt"
	"os"

	"gitlab.com/accumulatenetwork/accumulate/internal/core/execute"
	"gitlab.com/accumulatenetwork/accumulate/internal/database"
	"gitlab.com/accumulatenetwork/accumulate/internal/database/snapshot"
	"gitlab.com/accumulatenetwork/accumulate/internal/node/config"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

func main() {
	if len(os.Args) != 4 {
		fmt.Fprintln(os.Stderr, "usage: restore-snap <snapshot-file> <fresh-db-path> <partition>")
		os.Exit(2)
	}
	snapFile, dbPath, partition := os.Args[1], os.Args[2], os.Args[3]

	if err := os.MkdirAll(dbPath, 0700); err != nil {
		fmt.Fprintf(os.Stderr, "mkdir %s: %v\n", dbPath, err)
		os.Exit(1)
	}

	db, err := database.OpenBadger(dbPath, nil)
	if err != nil {
		fmt.Fprintf(os.Stderr, "open %s: %v\n", dbPath, err)
		os.Exit(1)
	}
	defer db.Close()
	db.SetObserver(execute.NewDatabaseObserver())

	f, err := os.Open(snapFile)
	if err != nil {
		fmt.Fprintf(os.Stderr, "open %s: %v\n", snapFile, err)
		os.Exit(1)
	}
	defer f.Close()

	scope := protocol.PartitionUrl(partition)
	netURL := config.NetworkUrl{URL: scope}
	if err := snapshot.FullRestore(db, f, nil, netURL); err != nil {
		fmt.Fprintf(os.Stderr, "restore: %v\n", err)
		os.Exit(1)
	}
	fmt.Fprintln(os.Stderr, "restore complete")
}
