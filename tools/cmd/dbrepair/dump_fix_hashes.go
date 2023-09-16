// Copyright 2023 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package main

import (
	"bytes"
	"fmt"
	"os"
	"sort"

	"github.com/spf13/cobra"
)

func runDumpFixHashes(_ *cobra.Command, args []string) {
	fixFile := args[0]
	outputFile := args[1]
	DumpFixHashes(fixFile, outputFile)
}


// Apply the fix file to a database
func DumpFixHashes(fixFile,outputFile string) (NumAdded uint64) {
	boldCyan.Println("\n Dump Fix hashes")

	f, err := os.Open(fixFile)
	checkf(err, "buildFix failed to open %s", fixFile)
	defer func() { _ = f.Close() }()

	o, err := os.Create(outputFile)
	checkf(err,"output file create failed %s",outputFile)

	var keyList [][]byte
	// Keys to be deleted

	NumAdded = read8(f, nil , "read key")
	for i := uint64(0); i < NumAdded; i++ {
		buff := make([]byte,32)
		read32(f, buff[:], "read key")
		keyList = append(keyList,buff)
	}
	fmt.Println()

	sort.Slice(keyList,func(i,j int)bool{return bytes.Compare(keyList[i], keyList[j])<0 })

	for _,key := range keyList{
		_,err := o.Write([]byte(fmt.Sprintf("%x\n",key)))		
		checkf(err,"write failed")
	}
	fmt.Printf("\nDone\n")
	return NumAdded
}
