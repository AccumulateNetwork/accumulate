// Copyright 2023 The Accumulate Authors
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

	"github.com/dgraph-io/badger"
	"github.com/spf13/cobra"
)

func runPrintDiff(_ *cobra.Command, args []string) {
	diffFile := args[0]
	goodDB := args[1]
	printDiff(diffFile, goodDB)
}

// print a Diff file
// Use a goodDB to pull the actual addresses
func printDiff(diffFile, goodDB string) {
	boldCyan.Println("\n PrintDiff")

	var AddedKeys [][]byte     // List of keys added to the bad state
	var ModifiedKeys [][8]byte // Map of keys and values modified from the good state

	var buff [32]byte // General buffer for reading entries
	f, err := os.Open(diffFile)
	checkf(err, "buildFix failed to open %s", diffFile)

	// Read an 8 byte, uint64 value and return it.
	// As a side effect, the first 8 bytes of buff hold the value
	read8 := func() uint64 {
		r, err := f.Read(buff[:8]) // Read 64 bits
		checkf(err, "failed to read count")
		if r != 8 {
			fatalf("failed to read a full 64 bit value")
		}
		return binary.BigEndian.Uint64(buff[:8])
	}

	read32 := func() {
		r, err := f.Read(buff[:32]) // Read 32
		checkf(err, "failed to read address")
		if r != 32 {
			fatalf("failed to read a full 64 bit value")
		}
	}
	// Load up the Diff file

	// Keys to be deleted
	NumAdded := read8()
	for i := uint64(0); i < NumAdded; i++ {
		read32()
		key := [32]byte{}
		copy(key[:], buff[:])
		AddedKeys = append(AddedKeys, key[:])
	}

	// Keys modified by the bad state
	NumModified := read8()
	for i := uint64(0); i < NumModified; i++ {
		read8()
		key := [8]byte{}
		copy(key[:], buff[:])
		ModifiedKeys = append(ModifiedKeys, key)
	}

	// Build a map of key hash prefixes to keys from the good DB
	db, close := OpenDB(goodDB)
	defer close()
	Hash2Key := buildHash2Key(db)

	fmt.Printf("%d Keys to delete:\n", len(AddedKeys))
	for _, k := range AddedKeys { // list all the keys added to the bad db
		fmt.Printf("   %x\n", k)
	}

	fmt.Printf("%d Keys to modify:\n", len(ModifiedKeys))
	for _, k := range ModifiedKeys { // list all the keys added to the bad db
		key, ok := Hash2Key[k]
		if !ok {
			fatalf("missing")
		}
		fmt.Printf("   %x ", key)

		err := db.View(func(txn *badger.Txn) error { // Get the value and write it
			item, err := txn.Get(key)
			checkf(err, "key/value failed to produce the value")
			err = item.Value(func(val []byte) error {
				vLen := len(val)
				if vLen > 40 {
					fmt.Printf("%x ... len %d\n", val[:40], vLen)
				} else {
					fmt.Printf("%x\n", val)
				}
				return nil
			})
			checkf(err, "in view")
			return nil
		})
		checkf(err, "Modified keys")
	}

}

func buildHash2Key(db *badger.DB) map[[8]byte][]byte {
	Hash2Key := make(map[[8]byte][]byte)

	var cnt int
	err := db.View(func(txn *badger.Txn) error {
		opts := badger.DefaultIteratorOptions
		opts.PrefetchValues = false // <= What this does is go through the keys in the db
		it := txn.NewIterator(opts) //    in whatever order is best for badger for speed
		defer it.Close()
		for it.Rewind(); it.Valid(); it.Next() {
			item := it.Item()
			k := *(*[32]byte)(item.Key())
			kh := sha256.Sum256(k[:])
			key := *(*[8]byte)(kh[:])
			if _, exists := Hash2Key[key]; exists {
				return fmt.Errorf("collision on %08x", key)
			}
			Hash2Key[key] = k[:]

			cnt++
			if cnt%100000 == 0 {
				print(".")
			}
		}
		return nil
	})
	println()
	checkf(err, "failed to collect all keys")
	fmt.Printf("\nkeys in db: %d\n", len(Hash2Key))

	return Hash2Key
}
