package main

import (
	"context"
	"fmt"
	"log"

	"gitlab.com/accumulatenetwork/accumulate/internal/api/v3"
	"gitlab.com/accumulatenetwork/accumulate/internal/database"
	"gitlab.com/accumulatenetwork/accumulate/pkg/api/v3"
	"gitlab.com/accumulatenetwork/accumulate/pkg/url"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

// Simple example using the API client (recommended approach)
func main() {
	// Connect to an Accumulate node
	client, err := api.New("https://mainnet.accumulatenetwork.io/v3")
	if err != nil {
		log.Fatalf("Failed to create client: %v", err)
	}

	// Look up a token account
	accountURL := url.MustParse("acc://alice/tokens")
	
	// Query the account state
	resp, err := client.Query(context.Background(), &api.DefaultQuery{
		Scope: accountURL,
	})
	if err != nil {
		log.Fatalf("Failed to query account: %v", err)
	}

	// Type assert to get the token account
	tokenAccount, ok := resp.Record.(*protocol.TokenAccount)
	if !ok {
		log.Fatalf("Not a token account: %T", resp.Record)
	}

	// Print account state
	fmt.Printf("Token Account: %s\n", tokenAccount.Url)
	fmt.Printf("Token: %s\n", tokenAccount.TokenUrl)
	fmt.Printf("Balance: %s\n", tokenAccount.Balance.String())

	// Query transaction history
	histResp, err := client.Query(context.Background(), &api.ChainQuery{
		Scope: accountURL,
		Name:  "main", // Main chain contains transaction history
		Range: &api.RangeOptions{
			Count:   10,      // Get last 10
			FromEnd: true,    // Start from the end
		},
	})
	if err != nil {
		log.Fatalf("Failed to query chain: %v", err)
	}

	// Print transactions
	fmt.Printf("\nLast %d transactions:\n", len(histResp.Records))
	for i, record := range histResp.Records {
		entry := record.Value.(*api.ChainEntryRecord[api.Record])
		fmt.Printf("\n[%d] Entry at index %d:\n", i+1, entry.Index)
		
		// The entry contains the transaction
		switch tx := entry.Value.Value.(type) {
		case *protocol.Transaction:
			printTransaction(tx)
		default:
			fmt.Printf("  Type: %T\n", tx)
		}
	}
}

func printTransaction(tx *protocol.Transaction) {
	fmt.Printf("  Principal: %s\n", tx.Header.Principal)
	fmt.Printf("  Initiator: %x\n", tx.Header.Initiator[:8])
	
	switch body := tx.Body.(type) {
	case *protocol.SendTokens:
		fmt.Printf("  Type: Send Tokens\n")
		for _, to := range body.To {
			fmt.Printf("    To: %s, Amount: %s\n", to.Url, to.Amount)
		}
		
	case *protocol.CreateTokenAccount:
		fmt.Printf("  Type: Create Token Account\n")
		fmt.Printf("    URL: %s\n", body.Url)
		fmt.Printf("    Token: %s\n", body.TokenUrl)
		
	case *protocol.BurnTokens:
		fmt.Printf("  Type: Burn Tokens\n")
		fmt.Printf("    Amount: %s\n", body.Amount)
		
	default:
		fmt.Printf("  Type: %s\n", body.Type())
	}
}