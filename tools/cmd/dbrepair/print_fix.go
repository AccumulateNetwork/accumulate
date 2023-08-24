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

func runPrintFix(_ *cobra.Command, args []string) {
	fixFile := args[0]
	printFix(fixFile)
}

// Apply the fix file to a database
func printFix(fixFile string) (cntModified, cntAdded int) {
	boldCyan.Println("\n Apply Fix")

	f, err := os.Open(fixFile)
	checkf(err, "buildFix failed to open %s", fixFile)
	defer func() { _ = f.Close() }()

	var buff [1024 * 1024]byte // A big buffer

	// Apply the fixes

	// Keys to be deleted
	NumAdded := read8(f, buff[:])
	var added [][]byte
	for i := uint64(0); i < NumAdded; i++ {
		read32(f, buff[:])
		added = append(added, append([]byte{}, buff[:32]...))
	}

	NumModified := read8(f, buff[:])
	type kv struct {addr []byte; value[]byte}
	var modified []kv
	for i := uint64(0); i < NumModified; i++ {
		keyLen := read8(f, buff[:])
		keyBuff := make([]byte,keyLen)
		read(f, keyBuff)

		valueLen := read8(f, buff[:])
		valueBuff := make([]byte,valueLen)
		read(f, valueBuff)
		
		modified = append(modified, kv {keyBuff,valueBuff })
	}

	fmt.Printf("\n%d Keys to be reverted to sync the db\n", len(modified))
	for _,v := range modified {
		fmt.Printf("%x,%x\n",v.addr,v.value)
	}

	fmt.Printf("\n%d Keys to be removed to sync the db\n", len(added))
	for _,v := range added {
		fmt.Printf("%x\n",v)
	}

	fmt.Printf("\nModified: %d Deleted: %d\n", NumModified, NumAdded)

	return int(NumModified), int(NumAdded)
}
