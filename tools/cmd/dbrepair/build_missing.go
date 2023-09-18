// Copyright 2023 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package main

import (
	"crypto/sha256"
	"errors"
	"fmt"
	"io"
	"os"

	"github.com/dgraph-io/badger"
	"github.com/spf13/cobra"
)

func runBuildMissing(_ *cobra.Command, args []string) {
	summary := args[0]
	badDB := args[1]
	diffFile := args[2]
	buildMissing(summary, badDB, diffFile)
}

// buildMissing
// Builds a diff file that solely looks for missing entries.  Since we
// already have a fix function, we simply need a fix file that does not
// remove anything.
//
// Diff file format:
//
//	N = 64 bits  -- number of Keys to be removed. Always zero
//
//	N = 64 bits  -- number of keys missing in the bad state
//	[N][8]bytes  -- keys of entries to restore to previous values
func buildMissing(summary, badDB, diffFile string) {
	boldCyan.Println("\n Build Missing")
	keys := make(map[[8]byte][8]byte)

	// Load up the keys from the good database
	s, err := os.Open(summary)
	checkf(err, "summary file failed to open: %v", err)
	defer func() { err := s.Close(); checkf(err, "summary file failed to close: %v", err) }()
	var cnt int
	for {
		kh := [8]byte{} // 8 bytes of the key hash
		vh := [8]byte{} // 8 bytes of the value hash
		_, err := s.Read(kh[:])
		if errors.Is(err, io.EOF) {
			break
		}
		_, err = s.Read(vh[:])
		check(err)
		keys[kh] = vh

		cnt++
		if cnt%100000 == 0 {
			print(".")
		}
	}
	println()

	// Collect the missing keys
	var missingKeys [][]byte // Slice of keys to update from the bad db

	// Open the Badger database to be fixed
	db, close := OpenDB(badDB)
	defer close()

	err = db.View(func(txn *badger.Txn) error {
		opts := badger.DefaultIteratorOptions
		opts.PrefetchValues = false // <= What this does is go through the keys in the db
		it := txn.NewIterator(opts) //    in whatever order is best for badger for speed
		defer it.Close()
		for it.Rewind(); it.Valid(); it.Next() {
			cnt++
			if cnt%100000 == 0 {
				print(".")
			}
			item := it.Item()
			k := *(*[32]byte)(item.Key())       //   Get the key and hash it.  IF the key isn't a
			kh := sha256.Sum256(k[:])           //   hash, then this takes care of that case.
			kb := [8]byte{}                     //   Get the first 8 bytes of the key hash
			copy(kb[:], kh[:8])                 //	    in an independent byte array
			if _, exists := keys[kb]; !exists { //   delete keys not in the summary
				continue //                          Ignore any new keys in the database to be fixed
			}

			delete(keys, kb) //                      Remove all keys that the database to fix has
		}
		return nil
	})
	println()
	checkf(err, "View of keys failed")

	for kh := range keys {
		kh := kh
		kb := [8]byte{}
		copy(kb[:], kh[:])
		missingKeys = append(missingKeys, kb[:]) // All the keys we did not find had to be added back
	}
	fmt.Printf("\nMissing: %d \n", len(missingKeys))

	// Write out the diff file, with only the keys missing from the good db
	f, err := os.Create(diffFile)
	checkf(err, "failed to open %s", diffFile)
	defer func() { _ = f.Close() }()

	write8(f, 0) //                       Number of keys to delete is always zero

	var percent float64
	write8(f, len(missingKeys))      //   Number of keys to add back
	for i, uk := range missingKeys { //   8 bytes of key hashes
		if float64(i)/float64(len(missingKeys)) >= percent*.99 {
			boldYellow.Printf("%02d%% ", int(percent*100))
			percent += .05
		}
		check2(f.Write(uk))
	}
}
