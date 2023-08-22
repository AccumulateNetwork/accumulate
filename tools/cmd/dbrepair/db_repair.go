package main

import (
	"crypto/sha256"
	"encoding/binary"
	"fmt"
	"os"
	"time"

	badger "github.com/dgraph-io/badger/v3"
	"github.com/spf13/cobra"
)

var cmdDbRepair = &cobra.Command{
	Use:   "dbRepair",
	Short: "utilities to repair bad databases using a good database",
}

var cmdBuildSummary = &cobra.Command{
	Use:   "buildSummary [badger database] [summary file]",
	Short: "Build a Summary file using a good database",
	Args:  cobra.ExactArgs(2),
	Run:   buildSummary,
}

func init() {
	cmd.AddCommand(cmdDbRepair)
	cmdDbRepair.AddCommand(
		cmdBuildSummary,
	)
}

func buildSummary(_ *cobra.Command, args []string) {
	dbName := args[1]
	dbSummary := args[2]

	// Open the Badger database that is the good one.
	db, err := badger.Open(badger.DefaultOptions(dbName))
	check(err)

	defer func() {
		if err := db.Close(); err != nil {
			fmt.Printf("error closing db %v\n", err)
		}
	}()

	summary, err := os.Create(dbSummary)
	check(err)

	start := time.Now()

	keys := make(map[uint64]bool)

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
					return err
				}
				if _, err := summary.Write(vh[:8]); err != nil {
					return err
				}
				return nil
			})
			check(err)
		}
		return nil
	})
	check(err)

	fmt.Printf("\nBuilding Summary file with %d keys took %v\n", len(keys), time.Since(start))

	fmt.Printf("\nValidating %d keys took %v\n", len(keys), time.Since(start))

}
