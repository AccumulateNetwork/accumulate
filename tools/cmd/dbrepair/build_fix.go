package main

import (
	"encoding/binary"
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

	f, err = os.Create(fixFile)
	checkf(err, "could not create the fixFile %s", fixFile)
	var bBuff [1024 * 1024]byte // A fixed buffer far larger than any entry

	write8 := func(v uint64) {
		binary.BigEndian.PutUint64(bBuff[:], v)
		i, err := f.Write(bBuff[:8])
		checkf(err, "failed a write to fix file %s", fixFile)
		if i != 8 {
			fatalf("failed to write the 8 bytes to %s", fixFile)
		}
	}
	write := func(d []byte) {
		i, err := f.Write(d)
		checkf(err, "failed a write to fix file %s", fixFile)
		if i != len(d) {
			fatalf("failed to make a complete write of %d. wrote %d", len(d), i)
		}
	}

	write8(uint64(len(AddedKeys)))
	for _, k := range AddedKeys { // list all the keys added to the bad db
		write(k)
	}

	write8(uint64(len(ModifiedKeys)))
	var kBuff [8]byte
	for _, k := range ModifiedKeys { // list all the keys added to the bad db
		copy(kBuff[:], k[:])
		key, ok := Hash2Key[kBuff]
		if !ok {
			fatalf("missing")
		}
		write8(uint64(len(key))) //                     Write the key length
		write(key)               //                     Write the key

		err := db.View(func(txn *badger.Txn) error { // Get the value and write it
			item, err := txn.Get([]byte(key))
			checkf(err, "key/value failed to produce the value")
			err = item.Value(func(val []byte) error {
				write8(uint64(len(val))) //             Write the value length
				write(val)               //             Write the value
				return nil
			})
			checkf(err, "in view")
			return nil
		})
		checkf(err, "Modified keys")
	}

}
