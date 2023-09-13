// Copyright 2023 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package main

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
)

func runApplyFix(_ *cobra.Command, args []string) {
	fixFile := args[0]
	badDB := args[1]
	applyFix(fixFile, badDB)
}

// Apply the fix file to a database
func applyFix(fixFile, badDB string) (NumModified, NumAdded uint64) {
	boldCyan.Println("\n Apply Fix")

	f, err := os.Open(fixFile)
	checkf(err, "buildFix failed to open %s", fixFile)
	defer func() { _ = f.Close() }()

	db, close := OpenDB(badDB)
	defer close()

	var buff [1024 * 1024]byte // A big buffer

	// Apply the fixes

	// Keys to be deleted
	txn := db.NewWriteBatch()
	NumAdded = read8(f, buff[:],"read key to delete")
	for i := uint64(0); i < NumAdded; i++ {
		read32(f, buff[:],"read key to delete")
		err := txn.Delete(copyBuf(buff[:32]))
		checkf(err, "failed to delete")
	}
	fmt.Println()

	var keyBuff [1024]byte
	NumModified = read8(f, buff[:],"read number modified")
	for i := uint64(0); i < NumModified; i++ {
		keyLen := read8(f, buff[:],"read key of modified key/value")
		read(f, keyBuff[:keyLen])
		valueLen := read8(f, buff[:],"read value of modified key/value")
		read(f, buff[:valueLen])
		err := txn.Set(copyBuf(keyBuff[:keyLen]), copyBuf(buff[:valueLen]))
		checkf(err, "failed to update a value in the database")
	}
	check(txn.Flush())

	fmt.Printf("\nModified: %d Deleted: %d\n", NumModified, NumAdded)
	return NumModified, NumAdded
}

func copyBuf(b []byte) []byte {
	c := make([]byte, len(b))
	copy(c, b)
	return c
}
