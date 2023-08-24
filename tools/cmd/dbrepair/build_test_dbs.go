// Copyright 2023 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package main

import (
	"fmt"
	"os"
	"strconv"
	"time"

	"github.com/dgraph-io/badger"
	"github.com/spf13/cobra"
)

func runBuildTestDBs(_ *cobra.Command, args []string) {
	numEntries, err := strconv.Atoi(args[0])
	check(err)
	if numEntries < 100 {
		fatalf("entries must be greater than 100")
	}
	GoodDBName := args[1]
	BadDBName := args[2]

	check(os.RemoveAll(GoodDBName))
	check(os.RemoveAll(BadDBName))

	buildTestDBs(numEntries, GoodDBName, BadDBName)
}

func buildTestDBs(numEntries int, GoodDBName, BadDBName string) {
	boldCyan.Println("\n Build Test DBs")
	gDB, gClose := OpenDB(GoodDBName)
	defer gClose()

	bDB, bClose := OpenDB(BadDBName)
	defer bClose()

	var rh1, mrh RandHash                     // rh adds the good key vale pairs
	mrh.SetSeed([]byte{1, 4, 67, 8, 3, 5, 7}) // mrh with a different seed, adds bad data

	start := time.Now()
	var total int
	var cntMod, cntDel, cntAdd int
	var lastPercent int

	fmt.Printf("\nGenerating databases with about %d entries:\n", numEntries)

	for i := 0; i < numEntries; i++ {
		key := rh1.Next()
		size := rh1.GetIntN(512) + 128
		total += size
		value := rh1.GetRandBuff(size)

		// Write the good entries
		err := gDB.Update(func(txn *badger.Txn) error {
			err := txn.Set(key, value)
			check(err)
			return nil
		})
		check(err)

		// Write the bad entries

		op := 0 // Match the good db
		pick := func(t bool, value int) int {
			if t {
				return value
			}
			return op
		}
		op = pick(i%1027 == 0, 1) // Modify a key value pair
		op = pick(i%713 == 0, 2)  // Delete a key value pair
		op = pick(i%303 == 0, 3)  // Add a key value pair

		// Write bad entries

		err = bDB.Update(func(txn *badger.Txn) error {
			switch op {
			case 1:
				value[0]++
				err := txn.Set(key, value) //        Modify a key value pair
				check(err)
				cntMod++
			case 2: //                               Delete a key value pair
				cntDel++
			default:
				err := txn.Set(key, value) //        Normal
				check(err)
			}
			return nil
		})
		check(err)
		if op == 3 {
			err = bDB.Update(func(txn *badger.Txn) error {
				err := txn.Set(mrh.Next(), value) //        Normal
				check(err)
				return nil
			})
			check(err)
			cntAdd++
		}

		percent := i * 100 / numEntries
		if percent > lastPercent {
			//	fmt.Printf(" %2d%%\n\033[F", percent)
			if percent%5 == 0 {
				fmt.Printf("%3d%%", percent)
			}
			lastPercent = percent
		}

	}
	fmt.Println(" 100%")
	fmt.Printf("\nFINAL: #keys: %d time: %v size: %d\n", numEntries, time.Since(start), total)
	fmt.Printf("\nThe test modified %d keys, deleted %d keys, and added %d keys.\n", cntMod, cntDel, cntAdd)
	fmt.Printf("\nAs far as a fix is concerned, the bad DB has:\n")
	fmt.Printf("Modified: %d Added: %d", cntMod+cntDel, cntAdd)
}
