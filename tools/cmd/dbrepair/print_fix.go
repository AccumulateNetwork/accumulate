// Copyright 2024 The Accumulate Authors
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

func runPrintFix(_ *cobra.Command, args []string) {
	fixFile := args[0]
	printFix(fixFile)
}

// Apply the fix file to a database
func printFix(fixFile string) {
	boldCyan.Println("\n Apply Fix")

	f, err := os.Open(fixFile)
	checkf(err, "buildFix failed to open %s", fixFile)
	defer func() { _ = f.Close() }()

	var buff [1024 * 1024]byte // A big buffer

	// Apply the fixes

	// Keys to be deleted
	NumAdded := read8(f, buff[:], "keys to delete")
	fmt.Printf("Keys to delete %d\n", NumAdded)
	for i := uint64(0); i < NumAdded; i++ {
		read32(f, buff[:], "key to be deleted")
		fmt.Printf("key: %x\n", buff[:32])
	}

	var keyBuff [1024]byte
	NumModified := read8(f, buff[:], "keys modified/restored")
	fmt.Printf("Keys to add/update %d\n", NumModified)
	for i := uint64(0); i < NumModified; i++ {
		keyLen := read8(f, buff[:], "key length")
		read(f, keyBuff[:keyLen])
		valueLen := read8(f, buff[:], "value length")
		read(f, buff[:valueLen])
		fmt.Printf("key: %x value len %d\n", buff[:keyLen], valueLen)
	}

	fmt.Printf("\nModified: %d Deleted: %d\n", NumModified, NumAdded)
}
