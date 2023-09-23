// Copyright 2023 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package main

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"sort"

	"github.com/spf13/cobra"
	"gitlab.com/accumulatenetwork/accumulate/pkg/types/record"
)

func runDumpFixHashes(_ *cobra.Command, args []string) {
	fixFile := args[0]
	outputFile := args[1]
	DumpFixHashes(fixFile, outputFile)
}

// Apply the fix file to a database
func DumpFixHashes(fixFile, outputFile string) (NumAdded uint64) {
	boldCyan.Println("\n Dump Fix hashes")

	f, err := os.Open(fixFile)
	checkf(err, "buildFix failed to open %s", fixFile)
	defer func() { _ = f.Close() }()

	o, err := os.Create(outputFile)
	checkf(err, "output file create failed %s", outputFile)

	var keyList [][]byte
	// Keys to be deleted

	NumAdded = read8(f, nil, "read delete")
	if NumAdded != 0 {
		fatalf("only dump missing entry fix files")
	}

	//import "gitlab.com/accumulatenetwork/accumulate/pkg/types/record"

	makeKey := func(hash [32]byte) [32]byte {
		return record.NewKey("Transaction", hash, "Main").Hash()
	}

	NumModified := read8(f, nil, "read modified")
	for i := uint64(0); i < NumModified; i++ {
		kLen := read8(f, nil, "read length of key")
		if kLen != 32 {
			fatalf("keys always have to be 32 bytes long")
		}
		var key [32]byte
		read32(f, key[:], "read key")           // get the key
		badgerKey := makeKey(key)               // make it badger style
		keyList = append(keyList, badgerKey[:]) // add badger style key to list

		vLen := read8(f, nil, "read length of value") // skip the value
		_, err := f.Seek(int64(vLen), io.SeekCurrent)
		checkf(err, "seek over value")
	}
	fmt.Println()

	sort.Slice(keyList, func(i, j int) bool { return bytes.Compare(keyList[i], keyList[j]) < 0 })

	for _, key := range keyList {
		_, err := o.Write([]byte(fmt.Sprintf("%x\n", key)))
		checkf(err, "write failed")
	}
	fmt.Printf("\nDone\n")
	return NumAdded
}
