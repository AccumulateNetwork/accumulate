// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
	"time"

	"gitlab.com/accumulatenetwork/accumulate/pkg/api/v3"
	"gitlab.com/accumulatenetwork/accumulate/pkg/api/v3/jsonrpc"
	"gitlab.com/accumulatenetwork/accumulate/pkg/build"
	"gitlab.com/accumulatenetwork/accumulate/pkg/errors"
	"gitlab.com/accumulatenetwork/accumulate/pkg/types/messaging"
	"gitlab.com/accumulatenetwork/accumulate/pkg/url"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
	"gitlab.com/accumulatenetwork/accumulate/test/wallet"
)

// Config holds initialization configuration.
type Config struct {
	WalletPath   string
	NodeURL      string
	BatchSize    int
	Concurrency  int
	InitialFunds uint64 // In ACME (will be converted to precision)
	SkipLite     bool
	SkipADIToken bool
	SkipADIData  bool
	DryRun       bool
}

// Stats tracks initialization progress.
type Stats struct {
	LiteCreated     atomic.Uint64
	LiteFailed      atomic.Uint64
	ADITokenCreated atomic.Uint64
	ADITokenFailed  atomic.Uint64
	ADIDataCreated  atomic.Uint64
	ADIDataFailed   atomic.Uint64
	StartTime       time.Time
	Errors          []string
}

// Initializer manages test data initialization.
type Initializer struct {
	config Config
	wallet *wallet.Wallet
	client *jsonrpc.Client
	stats  *Stats
}

func main() {
	config := parseFlags()

	init, err := NewInitializer(config)
	if err != nil {
		log.Fatalf("Failed to create initializer: %v", err)
	}

	if err := init.Run(context.Background()); err != nil {
		log.Fatalf("Initialization failed: %v", err)
	}
}

func parseFlags() Config {
	var config Config

	homeDir, _ := os.UserHomeDir()
	defaultWalletPath := filepath.Join(homeDir, ".accumulate", "test-wallet.json")

	flag.StringVar(&config.WalletPath, "wallet", defaultWalletPath, "Path to test wallet")
	flag.StringVar(&config.NodeURL, "node", "http://localhost:8080/v3", "Accumulate node URL")
	flag.IntVar(&config.BatchSize, "batch-size", 10, "Number of accounts to create per batch")
	flag.IntVar(&config.Concurrency, "concurrency", 5, "Number of concurrent workers")
	flag.Uint64Var(&config.InitialFunds, "initial-funds", 1000, "Initial ACME to fund each account")
	flag.BoolVar(&config.SkipLite, "skip-lite", false, "Skip lite account creation")
	flag.BoolVar(&config.SkipADIToken, "skip-adi-token", false, "Skip ADI token account creation")
	flag.BoolVar(&config.SkipADIData, "skip-adi-data", false, "Skip ADI data account creation")
	flag.BoolVar(&config.DryRun, "dry-run", false, "Preview what would be created without executing")
	flag.Parse()

	return config
}

// NewInitializer creates a new test data initializer.
func NewInitializer(config Config) (*Initializer, error) {
	// Load wallet
	w, err := wallet.Load(config.WalletPath)
	if err != nil {
		return nil, fmt.Errorf("load wallet: %w", err)
	}

	if w.Funder == nil {
		return nil, fmt.Errorf("wallet has no funder account")
	}

	// Create API client
	client := jsonrpc.NewClient(config.NodeURL)

	return &Initializer{
		config: config,
		wallet: w,
		client: client,
		stats: &Stats{
			StartTime: time.Now(),
			Errors:    make([]string, 0),
		},
	}, nil
}

// Run executes the initialization process.
func (i *Initializer) Run(ctx context.Context) error {
	log.Println("Starting test data initialization")
	log.Printf("Wallet: %s", i.config.WalletPath)
	log.Printf("Node: %s", i.config.NodeURL)
	log.Printf("Funder: %s", i.wallet.Funder.URL)

	if i.config.DryRun {
		log.Println("DRY RUN MODE - No accounts will be created")
		i.printPlan()
		return nil
	}

	// Verify funder account exists and has funds
	if err := i.verifyFunder(ctx); err != nil {
		return fmt.Errorf("verify funder: %w", err)
	}

	// Initialize accounts by type
	if !i.config.SkipLite {
		if err := i.initializeLiteAccounts(ctx); err != nil {
			return fmt.Errorf("initialize lite accounts: %w", err)
		}
	}

	if !i.config.SkipADIToken {
		if err := i.initializeADITokenAccounts(ctx); err != nil {
			return fmt.Errorf("initialize ADI token accounts: %w", err)
		}
	}

	if !i.config.SkipADIData {
		if err := i.initializeADIDataAccounts(ctx); err != nil {
			return fmt.Errorf("initialize ADI data accounts: %w", err)
		}
	}

	i.printSummary()
	return nil
}

// printPlan shows what would be created.
func (i *Initializer) printPlan() {
	fmt.Println("\nInitialization Plan:")
	fmt.Println("====================")

	if !i.config.SkipLite {
		liteAccounts := i.wallet.GetAccountsByType("lite")
		fmt.Printf("Lite Accounts: %d\n", len(liteAccounts))
		fmt.Printf("  Initial funds: %d ACME each\n", i.config.InitialFunds)
	}

	if !i.config.SkipADIToken {
		adiTokenAccounts := i.wallet.GetAccountsByType("adi-token")
		fmt.Printf("ADI Token Accounts: %d\n", len(adiTokenAccounts))
		fmt.Printf("  Initial funds: %d ACME each\n", i.config.InitialFunds)
		fmt.Printf("  Credits: 10000 per key page\n")
	}

	if !i.config.SkipADIData {
		adiDataAccounts := i.wallet.GetAccountsByType("adi-data")
		fmt.Printf("ADI Data Accounts: %d\n", len(adiDataAccounts))
		fmt.Printf("  Credits: 10000 per key page\n")
	}

	fmt.Println("\nBatch size:", i.config.BatchSize)
	fmt.Println("Concurrency:", i.config.Concurrency)
}

// verifyFunder checks that the funder account exists and has sufficient funds.
func (i *Initializer) verifyFunder(ctx context.Context) error {
	log.Println("Verifying funder account...")

	funderURL, err := url.Parse(i.wallet.Funder.URL)
	if err != nil {
		return fmt.Errorf("parse funder URL: %w", err)
	}

	// Query funder account
	resp, err := i.client.Query(ctx, funderURL, &api.DefaultQuery{})
	if err != nil {
		return fmt.Errorf("query funder account: %w", err)
	}

	// Check if it's a token account
	accountRecord, ok := resp.(*api.AccountRecord)
	if !ok {
		return fmt.Errorf("funder is not an account")
	}

	tokenAccount, ok := accountRecord.Account.(*protocol.TokenAccount)
	if !ok {
		return fmt.Errorf("funder is not a token account")
	}

	balance := tokenAccount.Balance.Int64() / protocol.AcmePrecision
	log.Printf("Funder balance: %d ACME", balance)

	// Estimate required funds
	totalAccounts := 0
	if !i.config.SkipLite {
		totalAccounts += i.wallet.CountByType("lite")
	}
	if !i.config.SkipADIToken {
		totalAccounts += i.wallet.CountByType("adi-token")
	}

	requiredFunds := uint64(totalAccounts) * i.config.InitialFunds
	log.Printf("Estimated required funds: %d ACME for %d accounts", requiredFunds, totalAccounts)

	if balance < int64(requiredFunds) {
		log.Printf("WARNING: Funder balance may be insufficient")
		log.Printf("  Have: %d ACME, Need: ~%d ACME", balance, requiredFunds)
	}

	return nil
}

// initializeLiteAccounts creates and funds lite accounts.
func (i *Initializer) initializeLiteAccounts(ctx context.Context) error {
	accounts := i.wallet.GetAccountsByType("lite")
	if len(accounts) == 0 {
		return nil
	}

	log.Printf("Initializing %d lite accounts...", len(accounts))

	// Process in batches
	for idx := 0; idx < len(accounts); idx += i.config.BatchSize {
		end := idx + i.config.BatchSize
		if end > len(accounts) {
			end = len(accounts)
		}

		batch := accounts[idx:end]
		if err := i.processBatch(ctx, batch, i.createLiteAccount); err != nil {
			log.Printf("Batch %d-%d failed: %v", idx, end, err)
			// Continue with next batch
		}

		log.Printf("Progress: %d/%d lite accounts created", i.stats.LiteCreated.Load(), len(accounts))

		// Rate limiting
		time.Sleep(100 * time.Millisecond)
	}

	log.Printf("Lite accounts complete: %d created, %d failed",
		i.stats.LiteCreated.Load(), i.stats.LiteFailed.Load())

	return nil
}

// initializeADITokenAccounts creates ADI token accounts.
func (i *Initializer) initializeADITokenAccounts(ctx context.Context) error {
	accounts := i.wallet.GetAccountsByType("adi-token")
	if len(accounts) == 0 {
		return nil
	}

	log.Printf("Initializing %d ADI token accounts...", len(accounts))

	for idx := 0; idx < len(accounts); idx += i.config.BatchSize {
		end := idx + i.config.BatchSize
		if end > len(accounts) {
			end = len(accounts)
		}

		batch := accounts[idx:end]
		if err := i.processBatch(ctx, batch, i.createADITokenAccount); err != nil {
			log.Printf("Batch %d-%d failed: %v", idx, end, err)
		}

		log.Printf("Progress: %d/%d ADI token accounts created", i.stats.ADITokenCreated.Load(), len(accounts))
		time.Sleep(100 * time.Millisecond)
	}

	log.Printf("ADI token accounts complete: %d created, %d failed",
		i.stats.ADITokenCreated.Load(), i.stats.ADITokenFailed.Load())

	return nil
}

// initializeADIDataAccounts creates ADI data accounts.
func (i *Initializer) initializeADIDataAccounts(ctx context.Context) error {
	accounts := i.wallet.GetAccountsByType("adi-data")
	if len(accounts) == 0 {
		return nil
	}

	log.Printf("Initializing %d ADI data accounts...", len(accounts))

	for idx := 0; idx < len(accounts); idx += i.config.BatchSize {
		end := idx + i.config.BatchSize
		if end > len(accounts) {
			end = len(accounts)
		}

		batch := accounts[idx:end]
		if err := i.processBatch(ctx, batch, i.createADIDataAccount); err != nil {
			log.Printf("Batch %d-%d failed: %v", idx, end, err)
		}

		log.Printf("Progress: %d/%d ADI data accounts created", i.stats.ADIDataCreated.Load(), len(accounts))
		time.Sleep(100 * time.Millisecond)
	}

	log.Printf("ADI data accounts complete: %d created, %d failed",
		i.stats.ADIDataCreated.Load(), i.stats.ADIDataFailed.Load())

	return nil
}

// processBatch processes a batch of accounts concurrently.
func (i *Initializer) processBatch(ctx context.Context, batch []*wallet.Account, createFn func(context.Context, *wallet.Account) error) error {
	var wg sync.WaitGroup
	semaphore := make(chan struct{}, i.config.Concurrency)

	for _, account := range batch {
		wg.Add(1)
		go func(acct *wallet.Account) {
			defer wg.Done()

			semaphore <- struct{}{}
			defer func() { <-semaphore }()

			if err := createFn(ctx, acct); err != nil {
				log.Printf("Failed to create %s: %v", acct.URL, err)
			}
		}(account)
	}

	wg.Wait()
	return nil
}

// createLiteAccount funds a lite account.
func (i *Initializer) createLiteAccount(ctx context.Context, account *wallet.Account) error {
	recipientURL, err := url.Parse(account.URL)
	if err != nil {
		i.stats.LiteFailed.Add(1)
		return fmt.Errorf("parse URL: %w", err)
	}

	funderURL, _ := url.Parse(i.wallet.Funder.URL)
	funderKey, _ := i.wallet.Funder.GetPrivateKey()

	// Send tokens to fund the lite account
	env, err := build.Transaction().For(funderURL).
		SendTokens(i.config.InitialFunds, protocol.AcmePrecisionPower).To(recipientURL).
		SignWith(funderURL).Version(1).Timestamp(nil).PrivateKey(funderKey).
		Done()
	if err != nil {
		i.stats.LiteFailed.Add(1)
		return fmt.Errorf("build transaction: %w", err)
	}

	if err := i.submitAndWait(ctx, env); err != nil {
		i.stats.LiteFailed.Add(1)
		return err
	}

	i.stats.LiteCreated.Add(1)
	return nil
}

// createADITokenAccount creates an ADI with a token account.
func (i *Initializer) createADITokenAccount(ctx context.Context, account *wallet.Account) error {
	acctURL, err := url.Parse(account.URL)
	if err != nil {
		i.stats.ADITokenFailed.Add(1)
		return fmt.Errorf("parse URL: %w", err)
	}

	adi := acctURL.RootIdentity()
	book := adi.JoinPath("book")
	page := book.JoinPath("1")

	funderURL, _ := url.Parse(i.wallet.Funder.URL)
	funderKey, _ := i.wallet.Funder.GetPrivateKey()
	acctKey, _ := account.GetPublicKey()

	// Create Identity
	env, err := build.Transaction().For(funderURL).
		CreateIdentity(adi).
		WithKey(acctKey, protocol.SignatureTypeED25519).
		WithKeyBook(book).
		SignWith(funderURL).Version(1).Timestamp(nil).PrivateKey(funderKey).
		Done()
	if err != nil {
		i.stats.ADITokenFailed.Add(1)
		return fmt.Errorf("build CreateIdentity: %w", err)
	}

	if err := i.submitAndWait(ctx, env); err != nil {
		i.stats.ADITokenFailed.Add(1)
		return fmt.Errorf("submit CreateIdentity: %w", err)
	}

	// Add credits
	env, err = build.Transaction().For(funderURL).
		AddCredits().WithOracle(0.05).Purchase(10000).To(page).
		SignWith(funderURL).Version(1).Timestamp(nil).PrivateKey(funderKey).
		Done()
	if err != nil {
		i.stats.ADITokenFailed.Add(1)
		return fmt.Errorf("build AddCredits: %w", err)
	}

	if err := i.submitAndWait(ctx, env); err != nil {
		i.stats.ADITokenFailed.Add(1)
		return fmt.Errorf("submit AddCredits: %w", err)
	}

	// Create token account
	acctPrivKey, _ := account.GetPrivateKey()
	env, err = build.Transaction().For(adi).
		CreateTokenAccount(acctURL).ForToken(protocol.AcmeUrl()).WithAuthority(book).
		SignWith(page).Version(1).Timestamp(nil).PrivateKey(acctPrivKey).
		Done()
	if err != nil {
		i.stats.ADITokenFailed.Add(1)
		return fmt.Errorf("build CreateTokenAccount: %w", err)
	}

	if err := i.submitAndWait(ctx, env); err != nil {
		i.stats.ADITokenFailed.Add(1)
		return fmt.Errorf("submit CreateTokenAccount: %w", err)
	}

	// Fund the account
	env, err = build.Transaction().For(funderURL).
		SendTokens(i.config.InitialFunds, protocol.AcmePrecisionPower).To(acctURL).
		SignWith(funderURL).Version(1).Timestamp(nil).PrivateKey(funderKey).
		Done()
	if err != nil {
		i.stats.ADITokenFailed.Add(1)
		return fmt.Errorf("build SendTokens: %w", err)
	}

	if err := i.submitAndWait(ctx, env); err != nil {
		i.stats.ADITokenFailed.Add(1)
		return fmt.Errorf("submit SendTokens: %w", err)
	}

	i.stats.ADITokenCreated.Add(1)
	return nil
}

// createADIDataAccount creates an ADI with a data account.
func (i *Initializer) createADIDataAccount(ctx context.Context, account *wallet.Account) error {
	acctURL, err := url.Parse(account.URL)
	if err != nil {
		i.stats.ADIDataFailed.Add(1)
		return fmt.Errorf("parse URL: %w", err)
	}

	adi := acctURL.RootIdentity()
	book := adi.JoinPath("book")
	page := book.JoinPath("1")

	funderURL, _ := url.Parse(i.wallet.Funder.URL)
	funderKey, _ := i.wallet.Funder.GetPrivateKey()
	acctKey, _ := account.GetPublicKey()

	// Create Identity
	env, err := build.Transaction().For(funderURL).
		CreateIdentity(adi).
		WithKey(acctKey, protocol.SignatureTypeED25519).
		WithKeyBook(book).
		SignWith(funderURL).Version(1).Timestamp(nil).PrivateKey(funderKey).
		Done()
	if err != nil {
		i.stats.ADIDataFailed.Add(1)
		return fmt.Errorf("build CreateIdentity: %w", err)
	}

	if err := i.submitAndWait(ctx, env); err != nil {
		i.stats.ADIDataFailed.Add(1)
		return fmt.Errorf("submit CreateIdentity: %w", err)
	}

	// Add credits
	env, err = build.Transaction().For(funderURL).
		AddCredits().WithOracle(0.05).Purchase(10000).To(page).
		SignWith(funderURL).Version(1).Timestamp(nil).PrivateKey(funderKey).
		Done()
	if err != nil {
		i.stats.ADIDataFailed.Add(1)
		return fmt.Errorf("build AddCredits: %w", err)
	}

	if err := i.submitAndWait(ctx, env); err != nil {
		i.stats.ADIDataFailed.Add(1)
		return fmt.Errorf("submit AddCredits: %w", err)
	}

	// Create data account
	acctPrivKey, _ := account.GetPrivateKey()
	env, err = build.Transaction().For(adi).
		CreateDataAccount(acctURL).WithAuthority(book).
		SignWith(page).Version(1).Timestamp(nil).PrivateKey(acctPrivKey).
		Done()
	if err != nil {
		i.stats.ADIDataFailed.Add(1)
		return fmt.Errorf("build CreateDataAccount: %w", err)
	}

	if err := i.submitAndWait(ctx, env); err != nil {
		i.stats.ADIDataFailed.Add(1)
		return fmt.Errorf("submit CreateDataAccount: %w", err)
	}

	i.stats.ADIDataCreated.Add(1)
	return nil
}

// submitAndWait submits a transaction and waits for it to complete.
func (i *Initializer) submitAndWait(ctx context.Context, env *messaging.Envelope) error {
	subs, err := i.client.Submit(ctx, env, api.SubmitOptions{})
	if err != nil {
		return fmt.Errorf("submit: %w", err)
	}

	for _, sub := range subs {
		if !sub.Success {
			return fmt.Errorf("submission failed: %s", sub.Message)
		}
	}

	// Get transaction ID for polling
	var txID *url.TxID
	for _, msg := range env.Messages {
		if tx, ok := msg.(*messaging.TransactionMessage); ok {
			txID = tx.Transaction.ID()
			break
		}
	}

	if txID == nil {
		time.Sleep(500 * time.Millisecond)
		return nil
	}

	// Poll for completion
	timeout := time.After(30 * time.Second)
	ticker := time.NewTicker(500 * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-timeout:
			return fmt.Errorf("timeout waiting for transaction")
		case <-ticker.C:
			resp, err := i.client.Query(ctx, txID.AsUrl(), &api.DefaultQuery{})
			if err != nil {
				continue
			}

			txRec, ok := resp.(*api.MessageRecord[messaging.Message])
			if !ok {
				continue
			}

			switch txRec.Status {
			case errors.Delivered:
				return nil
			case errors.Pending:
				continue
			default:
				if txRec.Error != nil {
					return fmt.Errorf("transaction failed: %v", txRec.Error)
				}
				continue
			}
		}
	}
}

// printSummary prints the final statistics.
func (i *Initializer) printSummary() {
	duration := time.Since(i.stats.StartTime)

	separator := ""
	for j := 0; j < 50; j++ {
		separator += "="
	}
	fmt.Println("\n" + separator)
	fmt.Println("Initialization Complete")
	fmt.Println(separator)
	fmt.Printf("Duration: %v\n\n", duration.Round(time.Second))

	fmt.Println("Results:")
	fmt.Printf("  Lite Accounts:      %d created, %d failed\n",
		i.stats.LiteCreated.Load(), i.stats.LiteFailed.Load())
	fmt.Printf("  ADI Token Accounts: %d created, %d failed\n",
		i.stats.ADITokenCreated.Load(), i.stats.ADITokenFailed.Load())
	fmt.Printf("  ADI Data Accounts:  %d created, %d failed\n",
		i.stats.ADIDataCreated.Load(), i.stats.ADIDataFailed.Load())

	total := i.stats.LiteCreated.Load() + i.stats.ADITokenCreated.Load() + i.stats.ADIDataCreated.Load()
	totalFailed := i.stats.LiteFailed.Load() + i.stats.ADITokenFailed.Load() + i.stats.ADIDataFailed.Load()
	fmt.Printf("\nTotal: %d created, %d failed\n", total, totalFailed)

	if totalFailed > 0 {
		fmt.Printf("\nWARNING: %d accounts failed to initialize\n", totalFailed)
		fmt.Println("Check logs for details")
	}

	// Save summary to file
	i.saveSummary(total, totalFailed, duration)
}

// saveSummary saves initialization summary to a JSON file.
func (i *Initializer) saveSummary(total, failed uint64, duration time.Duration) {
	summary := map[string]interface{}{
		"timestamp":   time.Now().Format(time.RFC3339),
		"duration":    duration.String(),
		"node":        i.config.NodeURL,
		"wallet":      i.config.WalletPath,
		"lite":        map[string]uint64{"created": i.stats.LiteCreated.Load(), "failed": i.stats.LiteFailed.Load()},
		"adi_token":   map[string]uint64{"created": i.stats.ADITokenCreated.Load(), "failed": i.stats.ADITokenFailed.Load()},
		"adi_data":    map[string]uint64{"created": i.stats.ADIDataCreated.Load(), "failed": i.stats.ADIDataFailed.Load()},
		"total":       total,
		"failed":      failed,
		"batch_size":  i.config.BatchSize,
		"concurrency": i.config.Concurrency,
	}

	data, _ := json.MarshalIndent(summary, "", "  ")
	summaryFile := "/tmp/init-test-data-summary.json"
	if err := os.WriteFile(summaryFile, data, 0644); err == nil {
		fmt.Printf("\nSummary saved to: %s\n", summaryFile)
	}
}
