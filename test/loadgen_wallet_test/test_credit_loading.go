package main

import (
	"context"
	"crypto/ed25519"
	"crypto/sha256"
	"fmt"
	"log"
	"time"

	"gitlab.com/accumulatenetwork/accumulate/loadgen/wallet"
	"gitlab.com/accumulatenetwork/accumulate/pkg/api/v3"
	"gitlab.com/accumulatenetwork/accumulate/pkg/api/v3/jsonrpc"
	"gitlab.com/accumulatenetwork/accumulate/pkg/url"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

// TestCreditLoadingProgram demonstrates loading credits into a lite account
func main() {
	log.Println("Starting Credit Loading Test Program")
	log.Println("=====================================")
	
	// Configuration
	serverURL := "http://localhost:26660/v3" // Adjust to your devnet URL
	ctx := context.Background()
	
	// Create JSON-RPC client
	client := jsonrpc.NewClient(serverURL)
	
	// Step 1: Create a funding account (lite identity)
	log.Println("\nStep 1: Creating funding account...")
	fundingAccount := createLiteIdentity("funding-account-seed")
	log.Printf("Funding account created: %s", fundingAccount.URL)
	log.Printf("Public key hash: %x", fundingAccount.PublicKeyHash)
	
	// Step 2: Create a target account (lite identity) 
	log.Println("\nStep 2: Creating target account...")
	targetAccount := createLiteIdentity("target-account-seed")
	log.Printf("Target account created: %s", targetAccount.URL)
	log.Printf("Public key hash: %x", targetAccount.PublicKeyHash)
	
	// Step 3: Request ACME from faucet for funding account
	log.Println("\nStep 3: Requesting ACME from faucet for funding account...")
	err := requestFromFaucet(ctx, client, fundingAccount.URL)
	if err != nil {
		log.Printf("Warning: Faucet request failed: %v", err)
		log.Println("Continuing anyway - account may already have funds")
	} else {
		log.Println("Faucet request submitted successfully")
		// Wait a bit for transaction to process
		time.Sleep(2 * time.Second)
	}
	
	// Step 4: Check funding account balance
	log.Println("\nStep 4: Checking funding account balance...")
	balance, err := checkACMEBalance(ctx, client, fundingAccount.URL)
	if err != nil {
		log.Printf("Failed to check balance: %v", err)
	} else {
		log.Printf("Funding account ACME balance: %d ACME", balance/protocol.AcmePrecision)
	}
	
	// Step 5: Check current credits on both accounts
	log.Println("\nStep 5: Checking current credit balances...")
	fundingCredits, err := checkCredits(ctx, client, fundingAccount.URL)
	if err != nil {
		log.Printf("Failed to check funding credits: %v", err)
	} else {
		log.Printf("Funding account credits: %d", fundingCredits)
	}
	
	targetCredits, err := checkCredits(ctx, client, targetAccount.URL)
	if err != nil {
		log.Printf("Failed to check target credits: %v", err)
	} else {
		log.Printf("Target account credits: %d", targetCredits)
	}
	
	// Step 6: Create and use CreditManager to add credits to target account
	log.Println("\nStep 6: Setting up CreditManager...")
	signer := &wallet.DefaultTransactionSigner{}
	creditManager := wallet.NewCreditManager(client, client, signer, fundingAccount)
	
	log.Println("Attempting to top up target account with credits...")
	err = creditManager.TopUpLiteAccount(ctx, targetAccount)
	if err != nil {
		log.Printf("Failed to top up credits: %v", err)
		log.Println("\nNote: This may fail if:")
		log.Println("  1. The funding account doesn't have enough ACME")
		log.Println("  2. The network is not available")
		log.Println("  3. The transaction signing is not properly implemented")
		log.Println("\nThe DefaultTransactionSigner needs full implementation for production use.")
	} else {
		log.Println("Credit top-up successful!")
		
		// Wait for transaction to process
		time.Sleep(2 * time.Second)
		
		// Check new credit balance
		newCredits, err := checkCredits(ctx, client, targetAccount.URL)
		if err != nil {
			log.Printf("Failed to check new credits: %v", err)
		} else {
			log.Printf("Target account new credit balance: %d", newCredits)
		}
	}
	
	// Step 7: Test wallet integration
	log.Println("\nStep 7: Testing wallet integration...")
	testWalletIntegration(ctx, client)
	
	log.Println("\n=====================================")
	log.Println("Credit Loading Test Complete")
	log.Println("\nSummary:")
	log.Println("- Created funding and target lite accounts")
	log.Println("- Demonstrated faucet request for ACME")
	log.Println("- Showed credit balance checking")
	log.Println("- Attempted credit transfer using CreditManager")
	log.Println("\nNote: Without key pages, we can only work with lite accounts.")
	log.Println("Key pages require ADI creation which is a more complex operation.")
}

// createLiteIdentity creates a deterministic lite identity from a seed
func createLiteIdentity(seed string) *wallet.LiteIdentity {
	// Generate deterministic seed
	seedBytes := sha256.Sum256([]byte(seed))
	
	// Generate ED25519 keys from seed
	privKey := ed25519.NewKeyFromSeed(seedBytes[:])
	pubKey := privKey.Public().(ed25519.PublicKey)
	
	// Create key hash
	keyHash := sha256.Sum256(pubKey)
	
	// Create lite URL
	liteURL := protocol.LiteAuthorityForKey(pubKey, protocol.SignatureTypeED25519)
	
	// Create Key structure
	key := &wallet.Key{
		Type:          protocol.SignatureTypeED25519,
		PublicKey:     pubKey,
		PrivateKey:    privKey,
		PublicKeyHash: keyHash[:],
	}
	
	// Create LiteIdentity
	return &wallet.LiteIdentity{
		URL:           liteURL,
		Key:           key,
		PublicKeyHash: keyHash[:20],
		Created:       true,
		LastUpdated:   time.Now(),
	}
}

// requestFromFaucet requests ACME from the faucet
func requestFromFaucet(ctx context.Context, client *jsonrpc.Client, account *url.URL) error {
	opts := api.FaucetOptions{
		Token: protocol.AcmeUrl(),
	}
	
	_, err := client.Faucet(ctx, account, opts)
	return err
}

// checkACMEBalance checks the ACME balance of an account
func checkACMEBalance(ctx context.Context, client *jsonrpc.Client, account *url.URL) (uint64, error) {
	// Build token URL for ACME
	tokenUrl := account.WithPath("/ACME")
	
	// Query token account
	query := &api.DefaultQuery{}
	resp, err := client.Query(ctx, tokenUrl, query)
	if err != nil {
		return 0, err
	}
	
	// Extract balance
	if accRecord, ok := resp.(*api.AccountRecord); ok && accRecord.Account != nil {
		switch acc := accRecord.Account.(type) {
		case *protocol.TokenAccount:
			return acc.Balance.Uint64(), nil
		case *protocol.LiteTokenAccount:
			return acc.Balance.Uint64(), nil
		default:
			return 0, fmt.Errorf("unexpected account type: %T", accRecord.Account)
		}
	}
	
	return 0, fmt.Errorf("account not found")
}

// checkCredits checks the credit balance of an account
func checkCredits(ctx context.Context, client *jsonrpc.Client, account *url.URL) (uint64, error) {
	query := &api.DefaultQuery{}
	resp, err := client.Query(ctx, account, query)
	if err != nil {
		return 0, err
	}
	
	// Extract credit balance
	if accRecord, ok := resp.(*api.AccountRecord); ok && accRecord.Account != nil {
		switch acc := accRecord.Account.(type) {
		case *protocol.LiteIdentity:
			return acc.CreditBalance, nil
		case *protocol.KeyPage:
			return acc.CreditBalance, nil
		default:
			return 0, fmt.Errorf("account type %T doesn't have credits", accRecord.Account)
		}
	}
	
	return 0, fmt.Errorf("account not found")
}

// testWalletIntegration demonstrates wallet integration
func testWalletIntegration(ctx context.Context, client *jsonrpc.Client) {
	log.Println("\nTesting Wallet integration...")
	
	// Create a wallet
	w := wallet.NewWalletWithSeed([]byte("test-wallet-seed"))
	
	// Create some lite accounts using wallet
	account1, err := w.CreateLiteAccount()
	if err != nil {
		log.Printf("Failed to create lite account 1: %v", err)
		return
	}
	log.Printf("Created lite account 1: %s", account1.URL)
	
	account2, err := w.CreateLiteAccount()
	if err != nil {
		log.Printf("Failed to create lite account 2: %v", err)
		return
	}
	log.Printf("Created lite account 2: %s", account2.URL)
	
	// Set funding account
	w.SetFundingAccount(account1)
	log.Printf("Set %s as funding account", account1.URL)
	
	// Get wallet stats
	stats := w.GetStats()
	log.Printf("Wallet stats: %+v", stats)
	
	// Demonstrate funding configuration (but don't start it without proper setup)
	config := &wallet.FundingConfig{
		ServerURL:        "http://localhost:26660/v3",
		TargetCredits:    1000,
		MaxFaucetRequest: 10000000,
		FaucetCooldown:   60 * time.Second,
		CheckInterval:    5 * time.Second,
	}
	log.Printf("Funding config: %+v", config)
	
	log.Println("Wallet integration test complete")
}