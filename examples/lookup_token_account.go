package main

import (
	"fmt"
	"log"
	"math/big"

	"gitlab.com/accumulatenetwork/accumulate/internal/database"
	"gitlab.com/accumulatenetwork/accumulate/pkg/database/keyvalue"
	"gitlab.com/accumulatenetwork/accumulate/pkg/database/keyvalue/badger"
	"gitlab.com/accumulatenetwork/accumulate/pkg/errors"
	"gitlab.com/accumulatenetwork/accumulate/pkg/url"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

func main() {
	// Example: Look up acc://alice/tokens account
	accountURL := url.MustParse("acc://alice/tokens")
	
	// Open the database (adjust path as needed)
	store, err := badger.Open("path/to/database")
	if err != nil {
		log.Fatalf("Failed to open database: %v", err)
	}
	defer store.Close()

	// Create a database instance
	db := database.New(store, nil)
	
	// Begin a read-only batch
	batch := db.Begin(false)
	defer batch.Discard()

	// Look up the account and print its state
	if err := printTokenAccountState(batch, accountURL); err != nil {
		log.Fatalf("Failed to get account state: %v", err)
	}

	// Print the last 10 transactions
	if err := printLastTransactions(batch, accountURL, 10); err != nil {
		log.Fatalf("Failed to get transactions: %v", err)
	}
}

func printTokenAccountState(batch *database.Batch, accountURL *url.URL) error {
	// Get the account from the database using the URL
	account := batch.Account(accountURL)
	
	// Retrieve the main state (the protocol.TokenAccount object)
	mainState, err := account.Main().Get()
	if err != nil {
		return fmt.Errorf("failed to get main state: %w", err)
	}
	
	// Type assert to TokenAccount
	tokenAccount, ok := mainState.(*protocol.TokenAccount)
	if !ok {
		return fmt.Errorf("account %v is not a token account (type: %T)", accountURL, mainState)
	}
	
	// Print account state
	fmt.Println("=== Token Account State ===")
	fmt.Printf("URL: %s\n", tokenAccount.Url)
	fmt.Printf("Token URL: %s\n", tokenAccount.TokenUrl)
	fmt.Printf("Balance: %s\n", formatTokenAmount(tokenAccount.Balance))
	
	// Print authorities
	fmt.Println("\nAuthorities:")
	for i, auth := range tokenAccount.Authorities {
		fmt.Printf("  %d. %s (Entry %d)\n", i+1, auth.Url, auth.Entry)
	}
	
	// Get and print the BPT hash for this account
	hash, err := account.Hash()
	if err != nil {
		fmt.Printf("Failed to get BPT hash: %v\n", err)
	} else {
		fmt.Printf("\nBPT Hash: %x\n", hash)
	}
	
	// Check for pending transactions
	pending, err := account.Pending().Get()
	if err == nil && len(pending) > 0 {
		fmt.Printf("\nPending Transactions: %d\n", len(pending))
		for _, txid := range pending {
			fmt.Printf("  - %s\n", txid)
		}
	}
	
	return nil
}

func printLastTransactions(batch *database.Batch, accountURL *url.URL, count int) error {
	account := batch.Account(accountURL)
	
	// Get the main chain (transaction history)
	mainChain, err := account.MainChain().Get()
	if err != nil {
		return fmt.Errorf("failed to get main chain: %w", err)
	}
	
	// Get chain height (total number of entries)
	height := mainChain.Height()
	fmt.Printf("\n=== Transaction History ===\n")
	fmt.Printf("Total transactions in chain: %d\n\n", height)
	
	if height == 0 {
		fmt.Println("No transactions found")
		return nil
	}
	
	// Calculate starting index for last N transactions
	start := int64(0)
	if height > int64(count) {
		start = height - int64(count)
	}
	
	// Iterate through the last N transactions
	for i := start; i < height; i++ {
		// Get the chain entry at index i
		entry, err := mainChain.Entry(i)
		if err != nil {
			fmt.Printf("Error getting entry %d: %v\n", i, err)
			continue
		}
		
		fmt.Printf("Transaction #%d:\n", i+1)
		fmt.Printf("  Chain Index: %d\n", i)
		fmt.Printf("  Transaction Hash: %x\n", entry.Hash)
		
		// Use the hash to look up the full transaction
		txHash := *(*[32]byte)(entry.Hash)
		transaction := batch.Transaction(txHash)
		
		// Get transaction details
		if err := printTransactionDetails(transaction, "  "); err != nil {
			fmt.Printf("  Error getting details: %v\n", err)
		}
		
		fmt.Println()
	}
	
	return nil
}

func printTransactionDetails(tx *database.Transaction, indent string) error {
	// Get the main transaction message
	msg, err := tx.Main().Get()
	if err != nil {
		if errors.Is(err, errors.NotFound) {
			// Transaction might be pruned or not available
			return fmt.Errorf("transaction data not found")
		}
		return fmt.Errorf("failed to get transaction: %w", err)
	}
	
	// Get transaction status
	status, err := tx.Status().Get()
	if err == nil {
		fmt.Printf("%sStatus: %s (Code: %d)\n", indent, status.Code, status.Code)
		if status.Error != nil {
			fmt.Printf("%sError: %s\n", indent, status.Error.Message)
		}
	}
	
	// Try to get the transaction type
	switch txn := msg.(type) {
	case *protocol.Transaction:
		fmt.Printf("%sType: %s\n", indent, txn.Body.Type())
		
		// Handle specific transaction types
		switch body := txn.Body.(type) {
		case *protocol.SendTokens:
			fmt.Printf("%sFrom: %s\n", indent, txn.Header.Principal)
			fmt.Printf("%sTo: %s\n", indent, body.To[0].Url)
			fmt.Printf("%sAmount: %s\n", indent, formatBigInt(body.To[0].Amount))
			
		case *protocol.CreateTokenAccount:
			fmt.Printf("%sCreating: %s\n", indent, body.Url)
			fmt.Printf("%sToken: %s\n", indent, body.TokenUrl)
			
		case *protocol.AcmeFaucet:
			fmt.Printf("%sFaucet to: %s\n", indent, body.Url)
			
		default:
			fmt.Printf("%sTransaction Type: %T\n", indent, body)
		}
		
	case *protocol.SystemGenesis:
		fmt.Printf("%sType: System Genesis\n", indent)
		
	case *protocol.SyntheticDepositTokens:
		fmt.Printf("%sType: Synthetic Deposit\n", indent)
		fmt.Printf("%sToken: %s\n", indent, txn.Token)
		fmt.Printf("%sAmount: %s\n", indent, formatBigInt(txn.Amount))
		
	default:
		fmt.Printf("%sMessage Type: %T\n", indent, msg)
	}
	
	// Get produced transactions (synthetic transactions created by this one)
	produced, err := tx.Produced().Get()
	if err == nil && len(produced) > 0 {
		fmt.Printf("%sProduced %d synthetic transaction(s)\n", indent, len(produced))
	}
	
	return nil
}

func formatBigInt(amount *big.Int) string {
	if amount == nil {
		return "0"
	}
	return amount.String()
}

func formatTokenAmount(amount big.Int) string {
	// For ACME with 8 decimal places
	// You'd need to know the token's precision to format correctly
	return amount.String()
}