package main

import (
	"encoding/binary"
	"os"

	"github.com/spf13/cobra"
)

func runApplyFix(_ *cobra.Command, args []string) {
	fixFile := args[0]
	badDB := args[1]
	applyFix(fixFile, badDB)
}

// Apply the fix file to a database
func applyFix(fixFile, badDB string) {
	boldCyan.Println("\n Apply Fix")

	f, err := os.Open(fixFile)
	checkf(err, "buildFix failed to open %s", fixFile)
	defer func() { _ = f.Close() }()

	db, close := OpenDB(badDB)
	defer close()

	var buff [1024 * 1024]byte // A big buffer
	// Read an 8 byte, uint64 value and return it.
	// As a side effect, the first 8 bytes of buff hold the value
	read8 := func() uint64 {
		r, err := f.Read(buff[:8]) // Read 64 bits
		checkf(err, "failed to read count")
		if r != 8 {
			fatalf("failed to read a full 8 bytes")
		}
		return binary.BigEndian.Uint64(buff[:8])
	}

	read32 := func() {
		r, err := f.Read(buff[:32]) // Read 32
		checkf(err, "failed to read address")
		if r != 32 {
			fatalf("failed to read a full address")
		}
	}

	read := func(buff []byte) {
		r, err := f.Read(buff) // Read 32
		checkf(err, "failed to read value")
		if r != len(buff) {
			fatalf("failed to read the full value")
		}
	}

	// Apply the fixes

	// Keys to be deleted
	NumAdded := read8()
	for i := uint64(0); i < NumAdded; i++ {
		read32()
		txn := db.NewTransaction(true)
		err := txn.Delete(buff[:32])
		checkf(err, "failed to delete")
	}

	var keyBuff [1024]byte
	NumModified := read8()
	for i := uint64(0); i < NumModified; i++ {
		keyLen := read8()
		read(keyBuff[:keyLen])
		valueLen := read8()
		read(buff[:valueLen])
		txn := db.NewTransaction(true)
		err := txn.Set(keyBuff[:keyLen], buff[:valueLen])
		checkf(err, "failed to update a value in the database")
	}
}
