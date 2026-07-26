package main

import (
	"encoding/hex"
	"fmt"
	"os"

	"gitlab.com/accumulatenetwork/accumulate/internal/database"
	"gitlab.com/accumulatenetwork/accumulate/pkg/url"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

func main() {
	if len(os.Args) < 2 {
		fmt.Println("Usage: check-acc-height <path-to-accumulate.db>")
		os.Exit(1)
	}

	db, err := database.OpenBadger(os.Args[1], nil)
	if err != nil {
		fmt.Printf("Error opening database: %v\n", err)
		os.Exit(1)
	}
	defer db.Close()

	batch := db.Begin(false)
	defer batch.Discard()

	// Try dn.acme/ledger first
	ledgerUrl, _ := url.Parse("dn.acme/ledger")
	ledger := batch.Account(ledgerUrl)
	
	var ledgerState *protocol.SystemLedger
	err = ledger.Main().GetAs(&ledgerState)
	if err != nil {
		fmt.Printf("Error reading dn.acme/ledger: %v\n", err)
	} else {
		fmt.Printf("dn.acme/ledger - Index: %d, Timestamp: %v\n", ledgerState.Index, ledgerState.Timestamp)
	}

	// Also check the BPT root hash
	hash, err := batch.GetBptRootHash()
	if err != nil {
		fmt.Printf("Error getting BPT root: %v\n", err)
	} else {
		fmt.Printf("BPT Root Hash: %s\n", hex.EncodeToString(hash[:]))
	}
}
