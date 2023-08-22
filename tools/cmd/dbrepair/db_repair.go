package main

import (
	"bufio"
	"crypto/sha256"
	"encoding/binary"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	badger "github.com/dgraph-io/badger/v3"
	"github.com/spf13/cobra"
)

var cmdBuildSummary = &cobra.Command{
	Use:   "buildSummary [badger database] [summary file]",
	Short: "Build a Summary file using a good database",
	Args:  cobra.ExactArgs(2),
	Run:   buildSummary,
}

var cmdBuildTestDBs = &cobra.Command{
	Use:   "buildTestDBs [number of entries] [good database] [bad database]",
	Short: "Number of entries must be greater than 100, should be greater than 5000",
	Args:  cobra.ExactArgs(3),
	Run:   buildTestDBs,
}

func init() {
	cmd.AddCommand(cmdBuildSummary)
	cmd.AddCommand(cmdBuildTestDBs)
}

func buildSummary(_ *cobra.Command, args []string) {
	dbName := args[0]
	dbSummary := args[1]

	// Open the Badger database that is the good one.
	db, err := badger.Open(badger.DefaultOptions(dbName))
	check(err)

	defer func() {
		if err := db.Close(); err != nil {
			fmt.Printf("error closing db %v", err)
		}
	}()

	summary, err := os.Create(dbSummary)
	check(err)
	defer func() {
		if err := summary.Close(); err != nil {
			fmt.Printf("error closing %v", dbSummary)
		}
	}()

	start := time.Now()

	keys := make(map[uint64]bool)
cnt := 0
	// Run through the keys in the fastest way in order to collect
	// all the key value pairs, hash them (keys too, in case any keys are not hashes)
	// Every entry in the database is reduced to a 64 byte key and a 64 byte value.
	err = db.View(func(txn *badger.Txn) error {
		opts := badger.DefaultIteratorOptions
		opts.PrefetchValues = false // <= What this does is go through the keys in the db
		it := txn.NewIterator(opts) //    in whatever order is best for badger for speed
		defer it.Close()
		for it.Rewind(); it.Valid(); it.Next() {
			item := it.Item()
			k := item.Key()                      //   Get the key and hash it.  IF the key isn't a
			kh := sha256.Sum256(k)               //   hash, then this takes care of that case.
			kb := binary.BigEndian.Uint64(kh[:]) //
			if _, exists := keys[kb]; exists {   //                   Check for collisions. Not likely but
				fmt.Printf("\nKey collision building summary\n") //   matching 8 bytes of a hash IS possible
				os.Exit(1)
			}
			keys[kb] = true
			err = item.Value(func(val []byte) error {
				vh := sha256.Sum256(val)
				if _, err := summary.Write(kh[:8]); err != nil {
					check(err)
				}
				if _, err := summary.Write(vh[:8]); err != nil {
					check(err)
				}
				cnt++
				return nil
			})
			check(err)
		}
		return nil
	})
	check(err)

	fmt.Printf("\nBuilding Summary file with %d %d keys took %v\n", cnt, len(keys), time.Since(start))

}

// YesNoPrompt asks yes/no questions using the label.
func YesNoPrompt(label string) bool {
	choices := "Y/N"

	r := bufio.NewReader(os.Stdin)
	var s string

	for {
		fmt.Fprintf(os.Stderr, "%s (%s) ", label, choices)
		s, _ = r.ReadString('\n')
		s = strings.TrimSpace(s)
		if s == "" {
			return false
		}
		s = strings.ToLower(s)
		if s == "y" || s == "yes" {
			return true
		}
		if s == "n" || s == "no" {
			return false
		}
	}
}

func buildTestDBs(_ *cobra.Command, args []string) {
	numEntries, err := strconv.Atoi(args[0])
	check(err)
	if numEntries < 100 {
		fatalf("entries must be greater than 100")
	}
	GoodDBName := args[1]
	BadDBName := args[2]

	if YesNoPrompt(
		fmt.Sprintf("Delete all files in folders [%s] and [%s]?",
			GoodDBName, BadDBName)) {
		os.Remove(GoodDBName)
		os.Remove(BadDBName)
	}

	gDB, err := badger.Open(badger.DefaultOptions(GoodDBName))
	if err != nil {
		fmt.Printf("error opening db %v\n", err)
		return
	}
	defer func() {
		if err := gDB.Close(); err != nil {
			fmt.Printf("error closing db %v\n", err)
		}
	}()

	bDB, err := badger.Open(badger.DefaultOptions(BadDBName))
	if err != nil {
		fmt.Printf("error opening db %v\n", err)
		return
	}
	defer func() {
		if err := bDB.Close(); err != nil {
			fmt.Printf("error closing db %v\n", err)
		}
	}()

	var rh1, mrh RandHash                     // rh adds the good key vale pairs
	mrh.SetSeed([]byte{1, 4, 67, 8, 3, 5, 7}) // mrh with a different seed, adds bad data

	start := time.Now()
	var total int

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
		op = pick(i%3027 == 0, 1) // Modify a key value pair
		op = pick(i%1003 == 0, 2) // Add a key value pair
		op = pick(i%2013 == 0, 3) // Delete a key value pair

		// Write bad entries

		err = bDB.Update(func(txn *badger.Txn) error {
			switch op {
			case 0:
				err := txn.Set(key, value) //        Normal
				check(err)
			case 1:
				value[0]++
				err := txn.Set(key, value) //        Modify a key value pair
				check(err)
			case 3: //                              Delete a key value pair
			}
			return nil
		})
		check(err)
		if op == 2 {
			err = bDB.Update(func(txn *badger.Txn) error {
				err := txn.Set(key, value) //        Normal
				check(err)
				return nil
			})
			check(err)
		}

		percent := i * 100 * 100 / numEntries
		if percent > lastPercent {
			fmt.Printf("%2d ", percent)
			lastPercent = percent
		}

	}
	fmt.Printf("\nFINAL: #keys: %d time: %v size: %d\n", numEntries, time.Since(start), total)
}
