package main

import (
	"fmt"
	"os"

	"gitlab.com/accumulatenetwork/accumulate/internal/database"
	"gitlab.com/accumulatenetwork/accumulate/pkg/url"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

func main() {
	if len(os.Args) < 2 {
		fmt.Println("Usage: db-block-height <path-to-accumulate.db>")
		os.Exit(1)
	}

	dbPath := os.Args[1]

	// Open the database
	db, err := database.OpenBadger(dbPath, nil)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to open database: %v\n", err)
		os.Exit(1)
	}
	defer db.Close()

	batch := db.Begin(false)
	defer batch.Discard()

	// Try to read the SystemLedger from different partitions
	partitions := []string{"Directory", "Cyclops", "Apollo", "Yutu", "Chandrayaan"}

	for _, part := range partitions {
		ledgerUrl := protocol.PartitionUrl(part).JoinPath(protocol.Ledger)
		ledger := batch.Account(ledgerUrl)

		var ledgerState *protocol.SystemLedger
		err := ledger.Main().GetAs(&ledgerState)
		if err == nil && ledgerState != nil {
			fmt.Printf("Partition: %s\n", part)
			fmt.Printf("  Block Index: %d\n", ledgerState.Index)
			fmt.Printf("  Timestamp: %s\n", ledgerState.Timestamp)
		}
	}

	// Try to read BPT root hash
	hash, err := batch.GetBptRootHash()
	if err != nil {
		fmt.Printf("BPT Root Hash: error - %v\n", err)
	} else {
		fmt.Printf("BPT Root Hash: %X\n", hash)
	}

	// Also try the URL approach
	dnLedger, _ := url.Parse("acc://dn.acme/ledger")
	if dnLedger != nil {
		ledger := batch.Account(dnLedger)
		var ledgerState *protocol.SystemLedger
		err := ledger.Main().GetAs(&ledgerState)
		if err == nil && ledgerState != nil {
			fmt.Printf("\nDN Ledger (acc://dn.acme/ledger):\n")
			fmt.Printf("  Block Index: %d\n", ledgerState.Index)
			fmt.Printf("  Timestamp: %s\n", ledgerState.Timestamp)
		}
	}

	bvnLedger, _ := url.Parse("acc://bvn-Cyclops.acme/ledger")
	if bvnLedger != nil {
		ledger := batch.Account(bvnLedger)
		var ledgerState *protocol.SystemLedger
		err := ledger.Main().GetAs(&ledgerState)
		if err == nil && ledgerState != nil {
			fmt.Printf("\nBVN Ledger (acc://bvn-Cyclops.acme/ledger):\n")
			fmt.Printf("  Block Index: %d\n", ledgerState.Index)
			fmt.Printf("  Timestamp: %s\n", ledgerState.Timestamp)
		}
	}
}
