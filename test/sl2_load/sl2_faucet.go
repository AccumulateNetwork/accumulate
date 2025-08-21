package sl2_load_test

import (
	"context"
	"fmt"
	"time"

	"gitlab.com/accumulatenetwork/accumulate/pkg/api/v3"
	"gitlab.com/accumulatenetwork/accumulate/pkg/api/v3/jsonrpc"
	"gitlab.com/accumulatenetwork/accumulate/pkg/url"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

// FundAccount uses the faucet to fund the given account
// It lazily initializes the client if it doesn't exist
func FundAccount(test *SL2Test, accountURL *url.URL) error {
	// Initialize client if not already created
	if test.client == nil {
		client, err := initializeClient()
		if err != nil {
			return fmt.Errorf("failed to initialize client: %w", err)
		}
		test.client = client
		fmt.Println("Client initialized for devnet")
	}
	
	// Call faucet with retry logic
	maxRetries := 3
	for attempt := 1; attempt <= maxRetries; attempt++ {
		err := callFaucet(test.client, accountURL)
		if err == nil {
			fmt.Printf("Successfully funded %s\n", accountURL)
			
			// Wait a bit for the transaction to settle
			time.Sleep(2 * time.Second)
			
			// Verify balance after funding
			balance, err := getAccountBalance(test.client, accountURL)
			if err != nil {
				// Account might not exist yet, that's ok on first funding
				if attempt == 1 {
					fmt.Printf("Account not yet created (this is normal on first funding)\n")
					return nil
				}
				return fmt.Errorf("failed to verify balance: %w", err)
			}
			
			fmt.Printf("Account balance: %d ACME tokens\n", balance)
			return nil
		}
		
		if attempt < maxRetries {
			waitTime := time.Duration(attempt) * 2 * time.Second
			fmt.Printf("Faucet attempt %d failed, retrying in %v: %v\n", attempt, waitTime, err)
			time.Sleep(waitTime)
		} else {
			return fmt.Errorf("failed to fund account after %d attempts: %w", maxRetries, err)
		}
	}
	
	return fmt.Errorf("unexpected error in funding logic")
}

// InitializeClient creates a new JSON-RPC client for the devnet
func InitializeClient() (*jsonrpc.Client, error) {
	return initializeClient()
}

// initializeClient creates a new JSON-RPC client for the devnet
func initializeClient() (*jsonrpc.Client, error) {
	// Use standard devnet endpoint
	endpoint := "http://127.0.0.1:26660/v3"
	
	client := jsonrpc.NewClient(endpoint)
	
	// Test connection
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	
	_, err := client.NetworkStatus(ctx, api.NetworkStatusOptions{})
	if err != nil {
		return nil, fmt.Errorf("failed to connect to devnet at %s: %w", endpoint, err)
	}
	
	return client, nil
}

// callFaucet calls the faucet endpoint to fund an account
func callFaucet(client *jsonrpc.Client, accountURL *url.URL) error {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	
	// Use the Faucet method directly
	submission, err := client.Faucet(ctx, accountURL, api.FaucetOptions{})
	if err != nil {
		return fmt.Errorf("faucet request failed: %w", err)
	}
	
	// Check submission status
	if submission != nil && submission.Status != nil {
		if submission.Status.Error != nil {
			return fmt.Errorf("faucet transaction error: %v", submission.Status.Error)
		}
		if submission.Status.TxID != nil {
			fmt.Printf("Faucet transaction ID: %x\n", submission.Status.TxID)
		}
	}
	
	return nil
}

// getAccountBalance queries the balance of a lite token account
func getAccountBalance(client *jsonrpc.Client, accountURL *url.URL) (int64, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	
	// Query the account
	resp, err := client.Query(ctx, accountURL, nil)
	if err != nil {
		return 0, fmt.Errorf("failed to query account: %w", err)
	}
	
	// Cast to AccountRecord
	accRecord, ok := resp.(*api.AccountRecord)
	if !ok {
		return 0, fmt.Errorf("unexpected response type: %T", resp)
	}
	
	// Check if it's a lite token account
	liteAccount, ok := accRecord.Account.(*protocol.LiteTokenAccount)
	if !ok {
		return 0, fmt.Errorf("account is not a lite token account: %T", accRecord.Account)
	}
	
	// Return balance in ACME tokens (1 ACME = 1e8 units)
	return int64(liteAccount.Balance.Int64() / 1e8), nil
}