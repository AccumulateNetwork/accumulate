// Copyright 2025 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package main

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"sort"

	cometLog "github.com/cometbft/cometbft/libs/log"
	"github.com/spf13/cobra"
	coredb "gitlab.com/accumulatenetwork/accumulate/internal/database"
	"gitlab.com/accumulatenetwork/accumulate/pkg/database"
	"gitlab.com/accumulatenetwork/accumulate/pkg/database/snapshot"
	"gitlab.com/accumulatenetwork/accumulate/pkg/url"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

var cmdExport = &cobra.Command{
	Use:   "export-snapshot <database-path> <output-file> <partition-url>",
	Short: "Export a genesis snapshot from an Accumulate database",
	Args:  cobra.ExactArgs(3),
	Run:   runExport,
}

func main() {
	if err := cmdExport.Execute(); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}

func runExport(cmd *cobra.Command, args []string) {
	dbPath := args[0]
	outputPath := args[1]
	partitionURL := args[2]

	// Parse partition URL
	partition, err := url.Parse(partitionURL)
	if err != nil {
		fatalf("invalid partition URL: %v", err)
	}

	fmt.Printf("Exporting snapshot from database: %s\n", dbPath)
	fmt.Printf("Partition: %s\n", partition)
	fmt.Printf("Output: %s\n", outputPath)

	// Open the database (try LevelDB first, fallback to Badger)
	logger := cometLog.NewNopLogger()
	dbFullPath := filepath.Join(dbPath, "accumulate.db")

	var db *coredb.Database

	// Try LevelDB first
	db, err = coredb.OpenLevelDB(dbFullPath, logger)
	if err != nil {
		// Fallback to Badger
		fmt.Printf("LevelDB failed (%v), trying Badger...\n", err)
		db, err = coredb.OpenBadger(dbFullPath, logger)
		if err != nil {
			fatalf("failed to open database with both LevelDB and Badger: %v", err)
		}
		fmt.Println("Using Badger database")
	} else {
		fmt.Println("Using LevelDB database")
	}
	defer db.Close()

	// Create output file
	f, err := os.Create(outputPath)
	if err != nil {
		fatalf("failed to create output file: %v", err)
	}
	defer f.Close()

	// Export the snapshot
	if err := exportSnapshot(db, f, partition); err != nil {
		fatalf("failed to export snapshot: %v", err)
	}

	fmt.Printf("✓ Snapshot exported successfully to %s\n", outputPath)
}

func exportSnapshot(db *coredb.Database, file *os.File, partition *url.URL) error {
	// Start the snapshot
	w, err := snapshot.Create(file)
	if err != nil {
		return fmt.Errorf("create snapshot: %w", err)
	}

	// Load the ledger
	batch := db.Begin(false)
	defer batch.Discard()

	var ledger *protocol.SystemLedger
	err = batch.Account(partition.JoinPath(protocol.Ledger)).Main().GetAs(&ledger)
	if err != nil {
		return fmt.Errorf("load system ledger: %w", err)
	}

	// Load the BPT root hash
	rootHash, err := batch.BPT().GetRootHash()
	if err != nil {
		return fmt.Errorf("get BPT root hash: %w", err)
	}

	fmt.Printf("Ledger index: %d\n", ledger.Index)
	fmt.Printf("BPT root hash: %x\n", rootHash)

	// Write the header
	err = w.WriteHeader(&snapshot.Header{
		RootHash:     rootHash,
		SystemLedger: ledger,
	})
	if err != nil {
		return fmt.Errorf("write header: %w", err)
	}

	// Open a records section
	records, err := w.OpenRecords()
	if err != nil {
		return fmt.Errorf("open records: %w", err)
	}

	// Iterate over the BPT and collect accounts
	var index []snapshot.RecordIndexEntry
	accountCount := 0
	it := batch.IterateAccounts()
	for it.Next() {
		account := it.Value()
		accountCount++

		if accountCount%10000 == 0 {
			fmt.Printf("Processed %d accounts...\n", accountCount)
		}

		// Collect the account's records
		err = records.Collect(account, snapshot.CollectOptions{
			Walk: database.WalkOptions{
				IgnoreIndices: true,
			},
			DidCollect: func(value database.Value, section, offset uint64) error {
				index = append(index, snapshot.RecordIndexEntry{
					Key:     value.Key().Hash(),
					Section: int(section),
					Offset:  offset,
				})
				return nil
			},
		})
		if err != nil {
			return fmt.Errorf("collect account: %w", err)
		}
	}
	if err := it.Err(); err != nil {
		return fmt.Errorf("iterate accounts: %w", err)
	}

	fmt.Printf("Total accounts: %d\n", accountCount)

	err = records.Close()
	if err != nil {
		return fmt.Errorf("close records: %w", err)
	}

	// Write the index
	x, err := w.OpenIndex()
	if err != nil {
		return fmt.Errorf("open index: %w", err)
	}

	// Sort index by key hash
	sort.Slice(index, func(i, j int) bool {
		a, b := index[i], index[j]
		return bytes.Compare(a.Key[:], b.Key[:]) < 0
	})

	fmt.Printf("Writing index with %d entries...\n", len(index))
	for _, e := range index {
		err = x.Write(e)
		if err != nil {
			return fmt.Errorf("write index entry: %w", err)
		}
	}

	if err := x.Close(); err != nil {
		return fmt.Errorf("close index: %w", err)
	}

	return nil
}

func fatalf(format string, args ...interface{}) {
	fmt.Fprintf(os.Stderr, "Error: "+format+"\n", args...)
	os.Exit(1)
}
