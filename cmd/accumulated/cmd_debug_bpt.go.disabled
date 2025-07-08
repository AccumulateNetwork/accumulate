package main

import (
	"encoding/hex"
	"fmt"
	"os"

	"github.com/spf13/cobra"
	"gitlab.com/accumulatenetwork/accumulate/internal/core/execute"
	"gitlab.com/accumulatenetwork/accumulate/internal/database"
	"gitlab.com/accumulatenetwork/accumulate/pkg/database/keyvalue/memory"
)

var cmdDebugBpt = &cobra.Command{
	Use:   "debug-bpt <snapshot-file>",
	Short: "Compute and display the BPT root hash from a snapshot file",
	Args:  cobra.ExactArgs(1),
	Run:   debugBpt,
}

func debugBpt(_ *cobra.Command, args []string) {
	snapshotFile := args[0]

	// Open the snapshot file
	file, err := os.Open(snapshotFile)
	checkf(err, "open snapshot file")
	defer file.Close()

	// Create temporary in-memory database for BPT computation
	store := memory.New(nil)
	db := database.New(store, nil)
	db.SetObserver(execute.NewDatabaseObserver())
	defer db.Close()

	// Restore the snapshot to compute BPT
	opts := &database.RestoreOptions{
		SkipHashCheck: true, // Skip hash check since we're computing it
	}
	
	err = database.Restore(db, file, opts)
	checkf(err, "restore snapshot")

	// Get the computed BPT root hash
	batch := db.Begin(false)
	defer batch.Discard()
	
	rootHash, err := batch.GetBptRootHash()
	checkf(err, "get BPT root hash")

	fmt.Printf("Computed BPT Root Hash: %x\n", rootHash)
	fmt.Printf("Root Hash (hex): %s\n", hex.EncodeToString(rootHash[:]))
	fmt.Printf("\nTo use this root hash in your snapshot header, update the RootHash field to:\n")
	fmt.Printf("RootHash: [32]byte{")
	for i, b := range rootHash {
		if i > 0 {
			fmt.Printf(", ")
		}
		fmt.Printf("0x%02x", b)
	}
	fmt.Printf("}\n")
}
