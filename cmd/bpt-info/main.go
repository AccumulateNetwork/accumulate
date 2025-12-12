// Copyright 2025 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package main

import (
	"crypto/sha256"
	"flag"
	"fmt"
	"os"
	"sort"

	cometLog "github.com/cometbft/cometbft/libs/log"
	"gitlab.com/accumulatenetwork/accumulate/internal/core/execute"
	"gitlab.com/accumulatenetwork/accumulate/internal/database"
	"gitlab.com/accumulatenetwork/accumulate/internal/logging"
	"log/slog"
)

func main() {
	var (
		dbPath     string
		dbType     string
		verbose    bool
		printFirst int
	)

	flag.StringVar(&dbPath, "db", "", "Path to database directory")
	flag.StringVar(&dbType, "type", "leveldb", "Database type: badger or leveldb")
	flag.BoolVar(&verbose, "verbose", false, "Print detailed BPT entry information")
	flag.IntVar(&printFirst, "print-first", 0, "Print first N entries sorted by key hash")
	flag.Parse()

	if dbPath == "" {
		fmt.Fprintf(os.Stderr, "Usage: %s -db <database-path> [-type leveldb|badger] [-verbose] [-print-first N]\n", os.Args[0])
		os.Exit(1)
	}

	// Create logger
	slogLogger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{
		Level: slog.LevelInfo,
	}))
	logger := (*logging.Slogger)(slogLogger)
	cometLogger := cometLog.NewNopLogger()

	// Open database
	var db *database.Database
	var err error
	fmt.Printf("Opening %s database at %s...\n", dbType, dbPath)
	switch dbType {
	case "leveldb":
		db, err = database.OpenLevelDB(dbPath, logger)
	case "badger":
		db, err = database.OpenBadger(dbPath, cometLogger)
	default:
		fmt.Fprintf(os.Stderr, "Unknown database type: %s (use 'leveldb' or 'badger')\n", dbType)
		os.Exit(1)
	}
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to open database: %v\n", err)
		os.Exit(1)
	}
	defer db.Close()

	// Set observer
	db.SetObserver(execute.NewDatabaseObserver())

	// Open a read batch
	batch := db.Begin(false)
	defer batch.Discard()

	// Get BPT info
	bpt := batch.BPT()

	// Get root hash
	rootHash, err := bpt.GetRootHash()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to get root hash: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("\nBPT Information:\n")
	fmt.Printf("  Root Hash: %x\n", rootHash)

	// Collect BPT entries
	type entry struct {
		keyHash [32]byte
		value   [32]byte
	}
	var entries []entry

	it := bpt.Iterate(1000)
	count := 0
	for it.Next() {
		for _, kv := range it.Value() {
			count++
			if verbose || printFirst > 0 {
				kh := kv.Key.Hash()
				var v [32]byte
				copy(v[:], kv.Value)
				entries = append(entries, entry{keyHash: kh, value: v})
			}
		}
	}
	if it.Err() != nil {
		fmt.Fprintf(os.Stderr, "Failed to iterate BPT: %v\n", it.Err())
		os.Exit(1)
	}

	fmt.Printf("  Entry Count: %d\n", count)

	// Compute a deterministic hash of all entries to verify they're identical
	if len(entries) > 0 {
		// Sort entries by key hash for deterministic ordering
		sort.Slice(entries, func(i, j int) bool {
			for k := 0; k < 32; k++ {
				if entries[i].keyHash[k] != entries[j].keyHash[k] {
					return entries[i].keyHash[k] < entries[j].keyHash[k]
				}
			}
			return false
		})

		// Compute combined hash
		h := sha256.New()
		for _, e := range entries {
			h.Write(e.keyHash[:])
			h.Write(e.value[:])
		}
		combinedHash := h.Sum(nil)
		fmt.Printf("  Entries Combined Hash: %x\n", combinedHash)

		// Print first N entries
		if printFirst > 0 {
			fmt.Printf("\nFirst %d entries (sorted by key hash):\n", printFirst)
			for i := 0; i < printFirst && i < len(entries); i++ {
				fmt.Printf("  %d: key=%x value=%x\n", i, entries[i].keyHash, entries[i].value)
			}
		}
	}

	fmt.Printf("\nNote: BPT uses Power parameter to determine tree branching factor.\n")
	fmt.Printf("      Default Power is 8. If source and restore have different Power,\n")
	fmt.Printf("      the tree structure (and thus root hash) will differ even with\n")
	fmt.Printf("      identical leaf values.\n")
	fmt.Printf("\n      The 'Entries Combined Hash' is computed from sorted entries\n")
	fmt.Printf("      and should be identical for databases with the same BPT content,\n")
	fmt.Printf("      regardless of tree structure.\n")
}
