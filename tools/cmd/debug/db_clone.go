// Copyright 2024 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package main

import (
	"fmt"
	"net"
	"time"

	"github.com/spf13/cobra"
	"gitlab.com/accumulatenetwork/accumulate/internal/database/record"
	"gitlab.com/accumulatenetwork/accumulate/pkg/database/keyvalue"
	"gitlab.com/accumulatenetwork/accumulate/pkg/database/keyvalue/remote"
)

var cmdDbClone = &cobra.Command{
	Use:   "clone [source] [destination]",
	Short: "Clone a database",
	Example: `` +
		`  debug db clone badger:///path/to/source.db leveldb:///path/to/dest.db` + "\n" +
		`  debug db clone badger:///path/to/source.db unix:///path/to/clone.sock` + "\n" +
		`  debug db clone unix:///path/to/clone.sock  leveldb:///path/to/dest.db` + "\n",
	Args: cobra.ExactArgs(2),
	Run:  cloneDatabases,
}

func init() {
	cmdDb.AddCommand(cmdDbClone)
}

func cloneDatabases(_ *cobra.Command, args []string) {
	srcDb, srcAddr := openDbUrl(args[0], false)
	dstDb, dstAddr := openDbUrl(args[1], true)

	var didProgress bool
	progress := func(s string) {
		if !didProgress {
			fmt.Println(s)
			didProgress = true
			return
		}
		fmt.Printf("\033[A\r\033[K%s\n", s)
	}

	switch {
	case srcDb == nil && dstDb == nil:
		fatalf("cloning from remote to remote is not supported")

	case dstDb == nil:
		// Serve for cloning
		serveDatabases(srcDb, dstAddr)

	case srcDb == nil:
		// Clone from remote
		defer func() { check(dstDb.Close()) }()
		check(cloneRemote(dstDb, srcAddr, progress))

	default:
		// Clone local
		defer func() { check(srcDb.Close()) }()
		defer func() { check(dstDb.Close()) }()
		src := srcDb.Begin(nil, false)
		defer src.Discard()
		check(cloneDb(dstDb, src, progress))
	}
}

func cloneRemote(db keyvalue.Beginner, addr net.Addr, progress func(string)) error {
	return withRemoteKeyValueStore(addr, func(src *remote.Store) error {
		return cloneDb(db, src, progress)
	})
}

func cloneDb(dst keyvalue.Beginner, src keyvalue.Store, progress func(string)) error {
	progress("Cloning...")
	t := time.NewTicker(time.Second / 2)
	defer t.Stop()

	batch := dst.Begin(nil, true)
	defer func() { batch.Discard() }()
	var count int
	err := src.ForEach(func(key *record.Key, value []byte) error {
		select {
		case <-t.C:
			progress(fmt.Sprintf("Cloning %d...", count))
			err := batch.Commit()
			if err != nil {
				return err
			}
			batch = dst.Begin(nil, true)
		default:
		}
		count++

		return batch.Put(key, value)
	})
	if err != nil {
		return err
	}

	return batch.Commit()
}
