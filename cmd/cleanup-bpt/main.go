// Copyright 2025 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package main

import (
	"flag"
	"fmt"
	"os"

	cometLog "github.com/cometbft/cometbft/libs/log"
	"gitlab.com/accumulatenetwork/accumulate/internal/core/execute"
	"gitlab.com/accumulatenetwork/accumulate/internal/database"
	"gitlab.com/accumulatenetwork/accumulate/pkg/errors"
)

func main() {
	var (
		dbPath  string
		dbType  string
		dryRun  bool
	)

	flag.StringVar(&dbPath, "db", "", "Path to database directory")
	flag.StringVar(&dbType, "type", "badger", "Database type: badger or leveldb")
	flag.BoolVar(&dryRun, "dry-run", true, "Only report orphaned entries, don't delete")
	flag.Parse()

	if dbPath == "" {
		fmt.Fprintf(os.Stderr, "Usage: %s -db <database-path> [-type badger|leveldb] [-dry-run=false]\n", os.Args[0])
		fmt.Fprintf(os.Stderr, "\nThis tool removes orphaned BPT entries (entries without corresponding account data).\n")
		fmt.Fprintf(os.Stderr, "\nBy default runs in dry-run mode. Use -dry-run=false to actually delete entries.\n")
		os.Exit(1)
	}

	// Open database
	var db *database.Database
	var err error
	fmt.Printf("Opening %s database at %s...\n", dbType, dbPath)
	switch dbType {
	case "leveldb":
		db, err = database.OpenLevelDB(dbPath, nil)
	case "badger":
		db, err = database.OpenBadger(dbPath, cometLog.NewNopLogger())
	default:
		fmt.Fprintf(os.Stderr, "Unknown database type: %s\n", dbType)
		os.Exit(1)
	}
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to open database: %v\n", err)
		os.Exit(1)
	}
	defer db.Close()

	db.SetObserver(execute.NewDatabaseObserver())

	// Get initial root hash
	batch := db.Begin(true)
	initialHash, err := batch.GetBptRootHash()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to get initial root hash: %v\n", err)
		batch.Discard()
		os.Exit(1)
	}
	fmt.Printf("Initial BPT root hash: %x\n", initialHash)

	// Iterate through BPT and find orphaned entries
	orphanedCount := 0
	validCount := 0
	var orphanedKeys []string

	it := batch.IterateAccounts()
	for it.Next() {
		account := it.Value()
		u := account.Url()

		// Try to get the account's main data
		_, err := account.Main().Get()
		if err != nil {
			if errors.Is(err, errors.NotFound) {
				orphanedCount++
				orphanedKeys = append(orphanedKeys, u.String())
				if orphanedCount <= 20 {
					fmt.Printf("  Orphaned: %s\n", u)
				}
			} else {
				fmt.Printf("  Error checking %s: %v\n", u, err)
			}
		} else {
			validCount++
		}
	}
	if it.Err() != nil {
		fmt.Fprintf(os.Stderr, "Error iterating accounts: %v\n", it.Err())
		batch.Discard()
		os.Exit(1)
	}

	fmt.Printf("\nSummary:\n")
	fmt.Printf("  Valid accounts: %d\n", validCount)
	fmt.Printf("  Orphaned entries: %d\n", orphanedCount)

	if orphanedCount == 0 {
		fmt.Printf("\nNo orphaned entries found. Database is clean.\n")
		batch.Discard()
		return
	}

	if orphanedCount > 20 {
		fmt.Printf("  ... and %d more orphaned entries\n", orphanedCount-20)
	}

	if dryRun {
		fmt.Printf("\nDry run mode. Use -dry-run=false to delete orphaned entries.\n")
		batch.Discard()
		return
	}

	// Delete orphaned entries
	fmt.Printf("\nDeleting %d orphaned BPT entries...\n", orphanedCount)

	// Need to re-iterate to delete (can't delete while iterating)
	batch.Discard()
	batch = db.Begin(true)

	deleted := 0
	it = batch.IterateAccounts()
	for it.Next() {
		account := it.Value()
		_, err := account.Main().Get()
		if errors.Is(err, errors.NotFound) {
			// Delete this BPT entry
			err = batch.BPT().Delete(account.Key())
			if err != nil {
				fmt.Printf("  Failed to delete %s: %v\n", account.Url(), err)
			} else {
				deleted++
			}
		}
	}
	if it.Err() != nil {
		fmt.Fprintf(os.Stderr, "Error during deletion: %v\n", it.Err())
		batch.Discard()
		os.Exit(1)
	}

	// Update BPT and commit
	err = batch.UpdateBPT()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to update BPT: %v\n", err)
		batch.Discard()
		os.Exit(1)
	}

	err = batch.Commit()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to commit: %v\n", err)
		os.Exit(1)
	}

	// Get new root hash
	batch = db.Begin(false)
	newHash, err := batch.GetBptRootHash()
	batch.Discard()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to get new root hash: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("\nDeleted %d orphaned entries.\n", deleted)
	fmt.Printf("New BPT root hash: %x\n", newHash)
	fmt.Printf("\nDatabase cleaned. You can now create a snapshot.\n")
}
