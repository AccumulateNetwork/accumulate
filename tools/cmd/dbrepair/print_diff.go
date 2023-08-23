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
	Hash2Key := make(map[[8]byte][]byte)

	db, close := OpenDB(goodDB)
	defer close()
	err = db.View(func(txn *badger.Txn) error {
		opts := badger.DefaultIteratorOptions
		opts.PrefetchValues = false // <= What this does is go through the keys in the db
		it := txn.NewIterator(opts) //    in whatever order is best for badger for speed
		defer it.Close()
		for it.Rewind(); it.Valid(); it.Next() {
			item := it.Item()
			keyBuff := [32]byte{}
			copy(keyBuff[:], item.Key())
			kh := sha256.Sum256(keyBuff[:]) //   hash, then this takes care of that case.
			key := [8]byte{}
			if _, exists := Hash2Key[key]; exists {
				return fmt.Errorf("collision on %08x", key)
			}
			copy(key[:], kh[:8])
			Hash2Key[key] = keyBuff[:]
		}
		return nil
	})
	checkf(err, "failed to collect all keys from %s", goodDB)
	fmt.Printf("\nkeys in db: %d\n", len(Hash2Key))
	fmt.Printf("\nModified: %d Added: %d\n", len(ModifiedKeys), len(AddedKeys))

	fmt.Printf("%d Keys to delete:\n", len(AddedKeys))
	for _, k := range AddedKeys { // list all the keys added to the bad db
		fmt.Printf("   %x\n", k)
	}

	fmt.Printf("%d Keys to modify:\n", len(ModifiedKeys))
	var kBuff [8]byte
	for _, k := range ModifiedKeys { // list all the keys added to the bad db
		copy(kBuff[:], k[:])
		key := Hash2Key[kBuff]
		fmt.Printf("   %x ", key)

		err := db.View(func(txn *badger.Txn) error { // Get the value and write it
			item, err := txn.Get([]byte(key))
			checkf(err, "key/value failed to produce the value")
			err = item.Value(func(val []byte) error {
				vLen := len(val)
				if vLen > 40 {
					fmt.Printf("%x ... len %d\n", val[:40], vLen)
				}
				return nil
			})
			checkf(err, "in view")
			return nil
		})
		checkf(err, "Modified keys")
	}

}
