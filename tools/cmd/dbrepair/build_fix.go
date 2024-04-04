// Copyright 2024 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package main

import (
	"fmt"
	"os"

	"github.com/dgraph-io/badger"
	"github.com/spf13/cobra"
)

func runBuildFix(_ *cobra.Command, args []string) {
	diffFile := args[0]
	goodDB := args[1]
	fixFile := args[2]
	buildFix(diffFile, goodDB, fixFile)
}

// buildFix
// Builds a Fix file that can be sent to a node that needs to back up a block (or more)
// This is done by taking their "bad" state (database) and applying a set of operations
// where keys are modified, added, or deleted to move the database from its bad state to
// the good state (prior to accepting the block we are reversing)
//
// The fix file format.  Keys are in DB format (and we won't assume a length)
//
//	   N = number keys to delete - 8 bytes
//	   N copies of
//	     {
//		      M = Len of key             - 8 bytes
//	       key                        - M bytes
//	     }
//
//	   N = number keys to restore/add - 8 bytes
//	   N copies of
//	     {
//			  M = Len of key             - 8 bytes
//	       key                        - M bytes
//	       L = Len of value           - 8 bytes
//	       value                      - L bytes
//	     }
func buildFix(diffFile, goodDB, fixFile string) {
	boldCyan.Println("\n Build Fix")

	var AddedKeys [][]byte     // List of keys added to the bad state
	var ModifiedKeys [][8]byte // Map of keys and values modified from the good state

	var buff [32]byte // General buffer for reading entries
	f, err := os.Open(diffFile)
	checkf(err, "buildFix failed to open %s", diffFile)

	// Load up the Diff file

	// Keys to be deleted
	NumAdded := read8(f, buff[:], "keys to be deleted")
	boldYellow.Printf("Number of keys to delete: %d %016x\n", NumAdded, NumAdded)
	for i := uint64(0); i < NumAdded; i++ {
		var key [32]byte
		read32(f, key[:], "key to be deleted")
		AddedKeys = append(AddedKeys, key[:])
	}

	// Keys modified by the bad state
	NumModified := read8(f, buff[:], "keys modified or deleted")
	boldYellow.Printf("Number of keys to update/restore: %d %016x\n", NumModified, NumModified)
	for i := uint64(0); i < NumModified; i++ {
		var key [8]byte
		read8(f, key[:], "key length")
		ModifiedKeys = append(ModifiedKeys, key)
	}

	// Build a map of key hash prefixes to keys from the good DB
	db, close := OpenDB(goodDB)
	defer close()
	Hash2Key := buildHash2Key(db)

	f, err = os.Create(fixFile)
	checkf(err, "could not create the fixFile %s", fixFile)

	write8(f, uint64(len(AddedKeys)))
	for _, k := range AddedKeys { // list all the keys added to the bad db
		write(f, k)
	}

	write8(f, uint64(len(ModifiedKeys)))

	var percent float64
	for i, k := range ModifiedKeys { // list all the keys added to the bad db
		if float64(i)/float64(len(ModifiedKeys)) > percent*.99 {
			boldYellow.Printf("%02d%% ", int(percent*100))
			percent += .05
		}
		key, ok := Hash2Key[k]
		if !ok {
			fatalf("missing")
		}
		write8(f, uint64(len(key))) //                     Write the key length
		write(f, key)               //                     Write the key

		err := db.View(func(txn *badger.Txn) error { // Get the value and write it
			item, err := txn.Get(key)
			checkf(err, "key/value failed to produce the value")
			err = item.Value(func(val []byte) error {
				write8(f, uint64(len(val))) //             Write the value length
				write(f, val)               //             Write the value
				return nil
			})
			checkf(err, "in view")
			return nil
		})
		checkf(err, "Modified keys")
	}
	fmt.Println()
}
