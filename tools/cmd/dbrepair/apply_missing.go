// Copyright 2023 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package main

import (
	"errors"
	"fmt"
	"io"
	"os"

	"github.com/dgraph-io/badger"
	"github.com/spf13/cobra"
)

func runApplyMissing(_ *cobra.Command, args []string) {
	fixFile := args[0]
	badDB := args[1]
	applyMissing(fixFile, badDB)
}

// Apply the fix file to a database
func applyMissing(fixFile, badDB string) (add,mod,miss uint64) {
	boldCyan.Println("\n Apply Missing")

	var AddedKeys, ModifiedKeys, MissingKeys uint64

	f, err := os.Open(fixFile)
	checkf(err, "buildFix failed to open %s", fixFile)
	defer func() { _ = f.Close() }()

	db, close := OpenDB(badDB)
	defer close()

	var buff [1024 * 1024]byte // A big buffer

	// Apply the fixes

	// Keys to be deleted
	txn := db.NewWriteBatch()
	AddedKeys = read8(f, buff[:], "read key to delete")
	f.Seek(int64(AddedKeys)*32, io.SeekCurrent)

	var keyBuff [1024]byte
	NumModified := read8(f, buff[:], "read number modified")
	for i := uint64(0); i < NumModified; i++ {
		keyLen := read8(f, buff[:], "read key of modified key/value")
		read(f, keyBuff[:keyLen])

		valueLen := read8(f, buff[:], "read value of modified key/value")
		err := db.View(func(txn *badger.Txn) error {
			_, err := txn.Get(keyBuff[:keyLen])
			return err
		})
		if errors.Is(err, badger.ErrKeyNotFound) { // If the Key is found
			MissingKeys++
			read(f, buff[:valueLen])                                            // read the value
			err := txn.Set(copyBuf(keyBuff[:keyLen]), copyBuf(buff[:valueLen])) // add to database
			checkf(err, "failed to update a value in the database")
		} else {
			ModifiedKeys++
			f.Seek(int64(valueLen), io.SeekCurrent) // skip value if the key is found
		}
	}
	check(txn.Flush())

	fmt.Printf("\nDidn't do anything with %d keys to be deleted, or %d keys to be modified.  Added %d keys",
		AddedKeys, ModifiedKeys, MissingKeys)
	return AddedKeys,ModifiedKeys,MissingKeys
}
