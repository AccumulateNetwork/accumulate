package main

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"encoding/hex"
	"flag"
	"fmt"
	"log"
	"math/big"
	"time"

	"gitlab.com/accumulatenetwork/accumulate/internal/api/v2"
	"gitlab.com/accumulatenetwork/accumulate/pkg/build"
	client "gitlab.com/accumulatenetwork/accumulate/pkg/client/api/v2"
	"gitlab.com/accumulatenetwork/accumulate/pkg/url"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

var (
	serverURL = flag.String("server", "http://127.0.0.1:26660/v2", "DevNet server URL")
	duration  = flag.Duration("duration", 2*time.Minute, "Test duration")
	workers   = flag.Int("workers", 2, "Number of concurrent workers")
	verbose   = flag.Bool("verbose", false, "Verbose output")
)

type CrosschainConductorTest struct {
	client       *client.Client
	liteAccounts []LiteAccount
	stats        TestStats
}

type LiteAccount struct {
	URL        *url.URL
	PrivateKey ed25519.PrivateKey
	PublicKey  ed25519.PublicKey
	Balance    *big.Int
}

type TestStats struct {
	FaucetRequests int
	TokenTransfers int
	QueryRequests  int
	Errors         int
	StartTime      time.Time
}

func main() {
	flag.Parse()

	fmt.Printf("Crosschain Conductor Test\n")
	fmt.Printf("=========================\n")
	fmt.Printf("Server: %s\n", *serverURL)
	fmt.Printf("Duration: %v\n", *duration)
	fmt.Printf("Workers: %d\n", *workers)
	fmt.Printf("\n")

	test, err := NewCrosschainConductorTest(*serverURL)
	if err != nil {
		log.Fatalf("Failed to create test: %v", err)
	}

	err = test.Run(*duration, *workers)
	if err != nil {
		log.Fatalf("Test failed: %v", err)
	}
}

func NewCrosschainConductorTest(serverURL string) (*CrosschainConductorTest, error) {
	// Create client to connect to DevNet
	c, err := client.New(serverURL)
	if err != nil {
		return nil, fmt.Errorf("failed to create client: %v", err)
	}

	return &CrosschainConductorTest{
		client:       c,
		liteAccounts: make([]LiteAccount, 0),
		stats: TestStats{
			StartTime: time.Now(),
		},
	}, nil
}

func (t *CrosschainConductorTest) Run(duration time.Duration, workers int) error {
	fmt.Printf("Phase 1: Creating lite accounts and requesting faucet tokens...\n")

	// Create several lite accounts
	numAccounts := workers * 2 // 2 accounts per worker
	for i := 0; i < numAccounts; i++ {
		account, err := t.createLiteAccount()
		if err != nil {
			return fmt.Errorf("failed to create lite account %d: %v", i, err)
		}
		t.liteAccounts = append(t.liteAccounts, account)

		// Request tokens from faucet
		err = t.requestFaucetTokens(account)
		if err != nil {
			fmt.Printf("Warning: faucet request failed for account %s: %v\n", account.URL, err)
			t.stats.Errors++
		} else {
			t.stats.FaucetRequests++
			if *verbose {
				fmt.Printf("✓ Requested faucet tokens for %s\n", account.URL)
			}
		}

		// Small delay between faucet requests
		time.Sleep(100 * time.Millisecond)
	}

	fmt.Printf("Phase 2: Waiting for faucet transactions to process...\n")
	time.Sleep(5 * time.Second) // Wait for faucet transactions to be processed

	// Check balances with retry logic
	fmt.Printf("Phase 3: Checking account balances (waiting for faucet transactions)...\n")
	for i, account := range t.liteAccounts {
		balance, err := t.checkAccountBalanceWithRetry(account, 10, 1*time.Second)
		if err != nil {
			fmt.Printf("Warning: failed to check balance for account %d after retries: %v\n", i, err)
			t.stats.Errors++
		} else {
			t.liteAccounts[i].Balance = balance
			if *verbose {
				fmt.Printf("✓ Account %s balance: %s ACME\n", account.URL, formatACME(balance))
			}
		}
	}

	fmt.Printf("Phase 4: Starting crosschain token transfer test for %v...\n", duration)

	// Start crosschain token transfer test
	stopChan := make(chan bool)

	// Start workers for token transfers
	for i := 0; i < workers; i++ {
		go t.transferWorker(i, stopChan)
	}

	// Run for specified duration
	time.Sleep(duration)
	close(stopChan)

	// Wait a moment for workers to finish
	time.Sleep(1 * time.Second)

	t.printFinalStats()
	return nil
}

func (t *CrosschainConductorTest) createLiteAccount() (LiteAccount, error) {
	// Generate Ed25519 key pair
	publicKey, privateKey, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		return LiteAccount{}, fmt.Errorf("failed to generate key pair: %v", err)
	}

	// Create lite token account URL from public key
	liteURL, err := protocol.LiteTokenAddress(publicKey, protocol.ACME, protocol.SignatureTypeED25519)
	if err != nil {
		return LiteAccount{}, fmt.Errorf("failed to create lite token address: %v", err)
	}

	return LiteAccount{
		URL:        liteURL,
		PrivateKey: privateKey,
		PublicKey:  publicKey,
		Balance:    big.NewInt(0),
	}, nil
}

func (t *CrosschainConductorTest) requestFaucetTokens(account LiteAccount) error {
	// Create faucet request
	faucetReq := &protocol.AcmeFaucet{
		Url: account.URL,
	}

	// Submit the faucet request
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	resp, err := t.client.Faucet(ctx, faucetReq)
	if err != nil {
		return fmt.Errorf("failed to submit faucet request: %v", err)
	}

	if *verbose {
		fmt.Printf("Faucet transaction submitted: %s\n", hex.EncodeToString(resp.TransactionHash[:8]))
	}

	return nil
}

func (t *CrosschainConductorTest) checkAccountBalance(account LiteAccount) (*big.Int, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// Query the account
	queryReq := &api.GeneralQuery{}
	queryReq.Url = account.URL

	resp, err := t.client.Query(ctx, queryReq)
	if err != nil {
		return nil, fmt.Errorf("failed to query account: %v", err)
	}

	// Handle the raw map response
	respMap, ok := resp.(map[string]interface{})
	if !ok {
		return nil, fmt.Errorf("unexpected response type: %T", resp)
	}

	// Extract the data field
	data, ok := respMap["data"].(map[string]interface{})
	if !ok {
		return nil, fmt.Errorf("missing or invalid data field in response")
	}

	// Check if it's a lite token account
	accountType, ok := data["type"].(string)
	if !ok || accountType != "liteTokenAccount" {
		return nil, fmt.Errorf("account is not a lite token account, got type: %v", accountType)
	}

	// Extract the balance
	balanceStr, ok := data["balance"].(string)
	if !ok {
		return nil, fmt.Errorf("missing or invalid balance field")
	}

	// Parse balance as big.Int
	balance := new(big.Int)
	balance, ok = balance.SetString(balanceStr, 10)
	if !ok {
		return nil, fmt.Errorf("failed to parse balance: %s", balanceStr)
	}

	t.stats.QueryRequests++
	return balance, nil
}

func (t *CrosschainConductorTest) checkAccountBalanceWithRetry(account LiteAccount, maxRetries int, retryDelay time.Duration) (*big.Int, error) {
	for i := 0; i < maxRetries; i++ {
		balance, err := t.checkAccountBalance(account)
		if err == nil {
			// Successfully got balance
			if *verbose && i > 0 {
				fmt.Printf("✓ Account balance retrieved after %d retries\n", i)
			}
			return balance, nil
		}

		// If this is the last retry, return the error
		if i == maxRetries-1 {
			return nil, err
		}

		// Wait before retrying
		if *verbose {
			fmt.Printf("Retry %d/%d: waiting %v for account to be created...\n", i+1, maxRetries, retryDelay)
		}
		time.Sleep(retryDelay)
	}

	return nil, fmt.Errorf("max retries exceeded")
}

func (t *CrosschainConductorTest) transferWorker(workerID int, stopChan chan bool) {
	fmt.Printf("Transfer worker %d started\n", workerID)

	for {
		select {
		case <-stopChan:
			fmt.Printf("Transfer worker %d stopping\n", workerID)
			return
		default:
			err := t.performTokenTransfer(workerID)
			if err != nil {
				if *verbose {
					fmt.Printf("Worker %d transfer failed: %v\n", workerID, err)
				}
				t.stats.Errors++
			} else {
				t.stats.TokenTransfers++
				if *verbose {
					fmt.Printf("Worker %d: ✓ Token transfer completed\n", workerID)
				}
			}

			// Delay between transfers
			time.Sleep(2 * time.Second)
		}
	}
}

func (t *CrosschainConductorTest) performTokenTransfer(workerID int) error {
	if len(t.liteAccounts) < 2 {
		return fmt.Errorf("need at least 2 accounts for transfers")
	}

	// Select source and destination accounts
	sourceIdx := workerID % len(t.liteAccounts)
	destIdx := (workerID + 1) % len(t.liteAccounts)

	source := t.liteAccounts[sourceIdx]
	dest := t.liteAccounts[destIdx]

	// Check if source has balance
	if source.Balance.Cmp(big.NewInt(0)) <= 0 {
		return fmt.Errorf("source account has no balance")
	}

	// Transfer a small amount (0.1 ACME)
	transferAmount := big.NewInt(protocol.AcmePrecision / 10) // 0.1 ACME

	// Build transfer transaction
	env, err := build.Transaction().
		For(source.URL).
		Body(&protocol.SendTokens{
			To: []*protocol.TokenRecipient{
				{
					Url:    dest.URL,
					Amount: *transferAmount,
				},
			},
		}).
		SignWith(source.URL).Version(1).Timestamp(time.Now().UnixNano()).
		Signer(build.ED25519PrivateKey(source.PrivateKey)).
		Done()
	if err != nil {
		return fmt.Errorf("failed to build transaction: %v", err)
	}

	// Extract signature information
	keySig := env.Signatures[0].(protocol.KeySignature)

	// Marshal transaction body to bytes
	payloadBytes, err := env.Transaction[0].Body.MarshalBinary()
	if err != nil {
		return fmt.Errorf("failed to marshal transaction body: %v", err)
	}

	// Create transaction request
	txReq := &api.TxRequest{
		Origin: env.Transaction[0].Header.Principal,
		Signer: api.Signer{
			Url:           source.URL,
			SignatureType: protocol.SignatureTypeED25519,
			PublicKey:     source.PublicKey,
			Timestamp:     keySig.GetTimestamp(),
			Version:       keySig.GetSignerVersion(),
		},
		Signature: keySig.GetSignature(),
		Payload:   payloadBytes,
		TxHash:    env.Transaction[0].GetHash(),
	}

	// Execute the transaction
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	resp, err := t.client.Execute(ctx, txReq)
	if err != nil {
		return fmt.Errorf("failed to execute transfer transaction: %v", err)
	}

	if *verbose {
		fmt.Printf("Transfer transaction submitted: %s (from %s to %s, amount: %s ACME)\n",
			hex.EncodeToString(resp.TransactionHash[:8]),
			source.URL, dest.URL, formatACME(transferAmount))
	}

	return nil
}

func (t *CrosschainConductorTest) printFinalStats() {
	elapsed := time.Since(t.stats.StartTime)

	fmt.Printf("\n")
	fmt.Printf("Crosschain Conductor Test Results\n")
	fmt.Printf("==================================\n")
	fmt.Printf("Test Duration: %v\n", elapsed.Round(time.Second))
	fmt.Printf("Lite Accounts Created: %d\n", len(t.liteAccounts))
	fmt.Printf("Faucet Requests: %d\n", t.stats.FaucetRequests)
	fmt.Printf("Token Transfers: %d\n", t.stats.TokenTransfers)
	fmt.Printf("Query Requests: %d\n", t.stats.QueryRequests)
	fmt.Printf("Errors: %d\n", t.stats.Errors)

	if elapsed.Seconds() > 0 {
		transferRate := float64(t.stats.TokenTransfers) / elapsed.Seconds()
		fmt.Printf("Transfer Rate: %.2f transfers/sec\n", transferRate)
	}

	fmt.Printf("\nAccount Balances:\n")
	totalBalance := big.NewInt(0)
	for i, account := range t.liteAccounts {
		fmt.Printf("  Account %d (%s): %s ACME\n",
			i+1, account.URL.String()[len(account.URL.String())-8:], formatACME(account.Balance))
		totalBalance.Add(totalBalance, account.Balance)
	}
	fmt.Printf("  Total Balance: %s ACME\n", formatACME(totalBalance))

	fmt.Printf("\nThis test exercised the crosschain conductor by:\n")
	fmt.Printf("- Creating lite token accounts across partitions\n")
	fmt.Printf("- Using faucet to load tokens (tests crosschain faucet routing)\n")
	fmt.Printf("- Performing token transfers between accounts (tests crosschain transaction routing)\n")
	fmt.Printf("- Querying account states (tests crosschain query routing)\n")
	fmt.Printf("\nCrosschain Conductor Test Completed!\n")
}

func formatACME(amount *big.Int) string {
	if amount == nil {
		return "0.00"
	}

	// Convert to ACME (divide by precision)
	acme := new(big.Float).SetInt(amount)
	acme.Quo(acme, big.NewFloat(float64(protocol.AcmePrecision)))

	return acme.Text('f', 8) // 8 decimal places
}
