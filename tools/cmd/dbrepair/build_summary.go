// Copyright 2024 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package main

import (
	"crypto/sha256"
	"encoding/binary"
	"fmt"
	"os"
	"time"

	"github.com/dgraph-io/badger"
	"github.com/fatih/color"
	"github.com/spf13/cobra"
)

var boldCyan = color.New(color.FgCyan, color.Bold)
var boldYellow = color.New(color.FgHiYellow, color.Bold)
var boldBlue = color.New(color.FgBlue, color.Bold)
var _, _, _ = boldCyan, boldYellow, boldBlue

func runBuildSummary(_ *cobra.Command, args []string) {
	dbName := args[0]
	dbSummary := args[1]
	buildSummary(dbName, dbSummary)
}

func buildSummary(dbName, dbSummary string) {
	boldCyan.Println("\n Build Summary")

	// Open the Badger database that is the good one.
	db, err := badger.Open(badger.DefaultOptions(dbName))
	check(err)
	defer func() { checkf(db.Close(), "error closing database") }()

	summary, err := os.Create(dbSummary)
	check(err)
	defer func() { checkf(summary.Close(), "error closing summary ") }()

	start := time.Now()

	keys := make(map[uint64]bool)
	cnt := 0
	// Run through the keys in the fastest way in order to collect
	// all the key value pairs, hash them (keys too, in case any keys are not hashes)
	// Every entry in the database is reduced to a 64 byte key and a 64 byte value.
	err = db.View(func(txn *badger.Txn) error {
		opts := badger.DefaultIteratorOptions
		opts.PrefetchValues = false // <= What this does is go through the keys in the db
		it := txn.NewIterator(opts) //    in whatever order is best for badger for speed
		defer it.Close()
		for it.Rewind(); it.Valid(); it.Next() {
			item := it.Item()
			k := item.Key()                      //   Get the key and hash it.  IF the key isn't a
			kh := sha256.Sum256(k)               //   hash, then this takes care of that case.
			kb := binary.BigEndian.Uint64(kh[:]) //
			if _, exists := keys[kb]; exists {   //                   Check for collisions. Not likely but
				fmt.Printf("\nKey collision building summary\n") //   matching 8 bytes of a hash IS possible
				os.Exit(1)
			}
			keys[kb] = true
			err = item.Value(func(val []byte) error {
				vh := sha256.Sum256(val)
				if _, err := summary.Write(kh[:8]); err != nil {
					checkf(err, "writing key")
				}
				if _, err := summary.Write(vh[:8]); err != nil {
					checkf(err, "writing value hash")
				}
				cnt++
				if cnt%100000 == 0 {
					print(".")
				}
				return nil
			})
			checkf(err, "on read of value")
		}
		return nil
	})
	checkf(err, "on viewing keys")

	fmt.Printf("\nBuilding Summary file with %d keys took %v\n", len(keys), time.Since(start))

}
