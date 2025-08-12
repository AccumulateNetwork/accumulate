package main

import (
	"fmt"
	"log"

	"gitlab.com/accumulatenetwork/accumulate/internal/database"
	"gitlab.com/accumulatenetwork/accumulate/pkg/database/keyvalue/badger"
	"gitlab.com/accumulatenetwork/accumulate/pkg/url"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

// Direct database access showing exactly how BPT entries map to account data
func main() {
	// The account we want to look up
	accountURL := url.MustParse("acc://alice/tokens")
	
	// Open the database
	store, err := badger.Open("/path/to/accumulate/data")
	if err != nil {
		log.Fatalf("Failed to open database: %v", err)
	}
	defer store.Close()

	// Create database instance
	db := database.New(store, nil)
	batch := db.Begin(false) // Read-only
	defer batch.Discard()

	// Step 1: The BPT contains the account key and hash
	// The account key is constructed from the URL
	account := batch.Account(accountURL)
	
	// Step 2: Get the BPT hash for this account
	// This is what's stored as the VALUE in the BPT
	bptHash, err := account.Hash()
	if err != nil {
		log.Fatalf("Failed to get BPT hash: %v", err)
	}
	fmt.Printf("BPT Entry:\n")
	fmt.Printf("  Key: %s\n", accountURL)
	fmt.Printf("  Value (Hash): %x\n\n", bptHash)

	// Step 3: Use the account key to retrieve the actual data
	// Component 1: Main State
	mainState, err := account.Main().Get()
	if err != nil {
		log.Fatalf("Failed to get main state: %v", err)
	}
	
	tokenAccount, ok := mainState.(*protocol.TokenAccount)
	if !ok {
		log.Fatalf("Not a token account: %T", mainState)
	}

	fmt.Printf("Account State (from BPT key lookup):\n")
	fmt.Printf("  URL: %s\n", tokenAccount.Url)
	fmt.Printf("  Token: %s\n", tokenAccount.TokenUrl)
	fmt.Printf("  Balance: %s\n", tokenAccount.Balance.String())
	fmt.Printf("  Authorities: %d\n\n", len(tokenAccount.Authorities))

	// Step 4: Component 3 of BPT hash - Chains
	// Get the main chain which contains transaction history
	mainChain, err := account.MainChain().Get()
	if err != nil {
		log.Fatalf("Failed to get main chain: %v", err)
	}

	height := mainChain.Height()
	fmt.Printf("Transaction Chain:\n")
	fmt.Printf("  Total Entries: %d\n", height)
	fmt.Printf("  Chain Anchor: %x\n\n", mainChain.Anchor())

	// Step 5: Get the last 10 transactions
	count := 10
	if height < int64(count) {
		count = int(height)
	}
	
	fmt.Printf("Last %d Transactions:\n", count)
	fmt.Println("=" + "="*50)
	
	for i := height - int64(count); i < height; i++ {
		// Get chain entry
		entry, err := mainChain.Entry(i)
		if err != nil {
			fmt.Printf("Error at index %d: %v\n", i, err)
			continue
		}

		fmt.Printf("\n[%d] Chain Index: %d\n", height-i, i)
		fmt.Printf("     Entry Hash: %x\n", entry.Hash[:16])

		// Step 6: Use the hash as a database key to get transaction
		// This demonstrates that the BPT value (hash) is ALSO a database key
		txHash := *(*[32]byte)(entry.Hash)
		transaction := batch.Transaction(txHash)
		
		// Get transaction message
		msg, err := transaction.Main().Get()
		if err != nil {
			fmt.Printf("     Transaction not found (may be pruned)\n")
			continue
		}

		// Get transaction status
		status, _ := transaction.Status().Get()
		
		// Print transaction details
		switch tx := msg.(type) {
		case *protocol.Transaction:
			fmt.Printf("     Type: %s\n", tx.Body.Type())
			fmt.Printf("     Principal: %s\n", tx.Header.Principal)
			if status != nil {
				fmt.Printf("     Status: %s\n", status.Code)
			}
			
			// Show transaction-specific details
			switch body := tx.Body.(type) {
			case *protocol.SendTokens:
				if len(body.To) > 0 {
					fmt.Printf("     → Sent to: %s\n", body.To[0].Url)
					fmt.Printf("     → Amount: %s\n", body.To[0].Amount)
				}
			case *protocol.SyntheticDepositTokens:
				fmt.Printf("     → Deposit Amount: %s\n", body.Amount)
			}
			
		case *protocol.SyntheticDepositTokens:
			fmt.Printf("     Type: Synthetic Deposit\n")
			fmt.Printf("     Amount: %s\n", tx.Amount)
			
		default:
			fmt.Printf("     Message Type: %T\n", msg)
		}
	}

	// Step 7: Show how sub-accounts work (Component 2: Directory)
	// For ADIs, the directory contains sub-account URLs
	directory, err := account.Directory().Get()
	if err == nil && len(directory) > 0 {
		fmt.Printf("\n\nSub-Accounts in Directory:\n")
		for _, subURL := range directory {
			// Each sub-account has its own BPT entry
			subAccount := batch.Account(subURL)
			subHash, _ := subAccount.Hash()
			fmt.Printf("  %s → BPT Hash: %x\n", subURL, subHash[:8])
		}
	}

	// Step 8: Show pending transactions (Component 4)
	pending, err := account.Pending().Get()
	if err == nil && len(pending) > 0 {
		fmt.Printf("\nPending Transactions: %d\n", len(pending))
		for _, txid := range pending {
			fmt.Printf("  %s\n", txid)
		}
	}
}