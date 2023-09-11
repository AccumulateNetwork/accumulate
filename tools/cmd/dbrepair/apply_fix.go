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
func applyFix(fixFile, badDB string) {
	boldCyan.Println("\n Apply Fix")

	f, err := os.Open(fixFile)
	checkf(err, "buildFix failed to open %s", fixFile)
	defer func() { _ = f.Close() }()

	db, close := OpenDB(badDB)
	defer close()

	var buff [1024 * 1024]byte // A big buffer

	// Apply the fixes

	// Keys to be deleted
	var percent float64
	NumAdded := read8(f, buff[:], "keys to delete")
	for i := uint64(0); i < NumAdded; i++ {
		if float64(i)/float64(NumAdded) >= percent*.99 {
			boldYellow.Printf("%02d%% ", int(percent*100))
			percent += .05
		}
		read32(f, buff[:], "key to be deleted")
		txn := db.NewTransaction(true)
		err := txn.Delete(buff[:32])
		checkf(err, "failed to delete")
		check(txn.Commit())
	}
	fmt.Println()

	percent = 0
	var keyBuff [1024]byte
	NumModified := read8(f, buff[:], "keys modified/restored")
	for i := uint64(0); i < NumModified; i++ {
		if float64(i)/float64(NumModified) >= percent*.99 {
			boldYellow.Printf("%02d%% ", int(percent*100))
			percent += .05
		}
		keyLen := read8(f, buff[:], "key length")
		read(f, keyBuff[:keyLen])
		valueLen := read8(f, buff[:], "value length")
		read(f, buff[:valueLen])
		txn := db.NewTransaction(true)
		err := txn.Set(keyBuff[:keyLen], buff[:valueLen])
		checkf(err, "failed to update a value in the database")
		check(txn.Commit())
	}
	fmt.Printf("\n\nModified: %d Deleted: %d\n", NumModified, NumAdded)
}
