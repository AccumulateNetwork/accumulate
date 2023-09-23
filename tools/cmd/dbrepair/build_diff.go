// Copyright 2023 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package main

import (
	"crypto/sha256"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"os"

	"github.com/dgraph-io/badger"
	"github.com/spf13/cobra"
)

func runBuildDiff(_ *cobra.Command, args []string) {
	summary := args[0]
	badDB := args[1]
	diffFile := args[2]
	buildDiff(summary, badDB, diffFile)
}

// buildDiff
// Builds a diff file on a node with a bad database because they accepted a bad block.
// The diffFile can be moved to a node with a good database to create
// a Fix File.  What we hope is that the Fix File will be complete
// enough to fix all nodes that need to be backed up (have a bad database)
//
// Diff file format:
//
//	N = 64 bits  -- number of Keys added to the bad state
//	[N][32]bytes -- keys to delete
//
//	N = 64 bits  -- number of keys modified or missing in the bad state
//	[N][8]bytes  -- keys of entries to restore to previous values
func buildDiff(summary, badDB, diffFile string) (NumModified, NumAdded int) {
	boldCyan.Println("\n Build Diff")
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

	// Collect the differences
	var addedKeys [][]byte    // Slice of keys to delete from the bad db
	var modifiedKeys [][]byte // Slice of keys to update from the bad db
	var cntDel int

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
				addedKeys = append(addedKeys, k[:])
				cntDel++
				continue
			}
			err = item.Value(func(val []byte) error {
				vh := sha256.Sum256(val)
				if keys[kb] != *(*[8]byte)(vh[:]) {
					modifiedKeys = append(modifiedKeys, kb[:]) // Revert keys that exist but value is changed
				}
				return nil
			})
			checkf(err, "read of value failed")
			delete(keys, kb) // Remove the keys from the good db that are found in the bad db
		}
		return nil
	})
	println()
	checkf(err, "View of keys failed")

	for kh := range keys {
		kh := kh
		kb := [8]byte{}
		copy(kb[:], kh[:])
		modifiedKeys = append(modifiedKeys, kb[:]) // All the keys we did not find had to be added back
	}
	fmt.Printf("\nModified: %d Added: %d \n", len(modifiedKeys), len(addedKeys))

	var buff [32]byte
	f, err := os.Create(diffFile)
	checkf(err, "failed to open %s", diffFile)
	defer func() { _ = f.Close() }()

	wrt64 := func(v uint64) {
		binary.BigEndian.PutUint64(buff[:], v)
		_, err := f.Write(buff[:8])
		check(err)
	}
	wrt64int := func(v int) {
		wrt64(uint64(v))
	}

	wrt64int(len(addedKeys))       //   Number of keys to delete
	for _, dk := range addedKeys { //   32 bytes each
		check2(f.Write(dk))
		if len(dk) != 32 {
			fatalf("Key is not a hash")
		}
	}
	wrt64int(len(modifiedKeys))       //   Number of keys to revert
	for _, uk := range modifiedKeys { //   8 bytes of key hashes
		check2(f.Write(uk))
	}

	return len(modifiedKeys), len(addedKeys)
}
