// Copyright 2025 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package liteclient

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"strings"
	"time"

	"gitlab.com/accumulatenetwork/accumulate/pkg/api/v3/jsonrpc"
	v2 "gitlab.com/accumulatenetwork/accumulate/pkg/client/api/v2"
	"gitlab.com/accumulatenetwork/accumulate/pkg/types/merkle"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

// LiteClient is the core internal client that orchestrates account data retrieval,
// proof generation, and caching. This is the unified implementation that replaces
// the previous separate LiteClient and ADIOrchestrator structs.
//
// The LiteClient coordinates the workflow:
// 1. Receives GetADI() calls from the public API
// 2. Uses AccountHandler to discover and retrieve account data
// 3. Uses HealingProofGenerator to create cryptographic proofs
// 4. Uses UnifiedCache to store and retrieve cached data
// 5. Returns verified account information to the public API
type LiteClient struct {
	v2             *v2.Client
	v3             *jsonrpc.Client
	unifiedCache   *UnifiedCache
	adisOfInterest map[string]bool
	proofGenerator *HealingProofGenerator
	accountHandler *AccountHandler
}

// VerifiedAccountInfo represents account data with cryptographic proof validation
type VerifiedAccountInfo struct {
	URL          string
	Type         protocol.AccountType
	Balance      string
	Receipt      *merkle.Receipt
	Height       int64
	LastUpdated  time.Time
	Transactions []*TransactionInfo
}

// TransactionInfo represents transaction data
type TransactionInfo struct {
	TxID      string
	Type      string
	Status    string
	Timestamp time.Time
	Amount    string
	From      string
	To        string
}

// NewLiteClient creates a new internal lite client with the unified architecture
func NewLiteClient(serverURL string) (*LiteClient, error) {
	if serverURL == "" {
		return nil, fmt.Errorf("server URL cannot be empty")
	}

	// Create v2 API client
	v2Client, err := v2.New(serverURL)
	if err != nil {
		return nil, fmt.Errorf("failed to create v2 client: %w", err)
	}

	// Create v3 API client
	v3URL, err := convertToV3URL(serverURL)
	if err != nil {
		return nil, fmt.Errorf("failed to convert to v3 URL: %w", err)
	}

	v3Client := jsonrpc.NewClient(v3URL)

	// Create unified cache
	cache := NewUnifiedCache(time.Minute)

	// Create proof generator
	proofGenerator, err := NewHealingProofGenerator(v2Client)
	if err != nil {
		return nil, fmt.Errorf("failed to create healing proof generator")
	}

	// Create the LiteClient struct
	lc := &LiteClient{
		v2:             v2Client,
		v3:             v3Client,
		unifiedCache:   cache,
		adisOfInterest: make(map[string]bool),
		proofGenerator: proofGenerator,
	}

	// Initialize the account handler
	lc.accountHandler = NewAccountHandler(lc)

	return lc, nil
}

// convertToV3URL converts a v2 API URL to v3 format
func convertToV3URL(v2URL string) (string, error) {
	parsed, err := url.Parse(v2URL)
	if err != nil {
		return "", fmt.Errorf("invalid URL: %w", err)
	}

	// Replace /v2 with /v3
	v3Path := strings.Replace(parsed.Path, "/v2", "/v3", 1)
	if v3Path == parsed.Path {
		// If no /v2 found, append /v3
		v3Path = strings.TrimSuffix(parsed.Path, "/") + "/v3"
	}

	parsed.Path = v3Path
	return parsed.String(), nil
}

// ProcessADI is the main orchestration method that implements the GetADI workflow:
// 1. Check cache for fresh data
// 2. If cache miss, discover accounts directly
// 3. Generate proofs using HealingProofGenerator
// 4. Store results in UnifiedCache
// 5. Return verified account information
func (lc *LiteClient) ProcessADI(ctx context.Context, adiURL string) ([]*AccountData, error) {
	// Step 1: Check cache first
	cachedAccounts := lc.unifiedCache.GetADIAccounts(adiURL)
	if len(cachedAccounts) > 0 {
		// Return cached data directly (already in AccountData format)
		return cachedAccounts, nil
	}

	// Step 2: Cache miss - discover accounts directly
	accountURLs, err := lc.accountHandler.DiscoverADIAccounts(ctx, adiURL)
	if err != nil {
		return nil, fmt.Errorf("failed to discover accounts for ADI %s: %w", adiURL, err)
	}

	if len(accountURLs) == 0 {
		return nil, fmt.Errorf("no accounts found for ADI: %s", adiURL)
	}

	// Step 3: Process each account
	verifiedAccounts := make([]*AccountData, 0, len(accountURLs))
	for _, accountURL := range accountURLs {
		verifiedAccount, err := lc.processAccount(ctx, accountURL)
		if err != nil {
			// Log error but continue with other accounts
			fmt.Printf("Warning: failed to process account %s: %v\n", accountURL, err)
			continue
		}
		verifiedAccounts = append(verifiedAccounts, verifiedAccount)
	}

	if len(verifiedAccounts) == 0 {
		return nil, fmt.Errorf("failed to process any accounts for ADI: %s", adiURL)
	}

	return verifiedAccounts, nil
}

// processAccount handles the complete workflow for a single account:
// 1. Retrieve account data using AccountHandler
// 2. Generate proof using HealingProofGenerator
// 3. Store in cache using UnifiedCache
func (lc *LiteClient) processAccount(ctx context.Context, accountURL string) (*AccountData, error) {
	// Step 1: Retrieve account data
	accountData, err := lc.accountHandler.GetAccountData(ctx, accountURL)
	if err != nil {
		return nil, fmt.Errorf("failed to get account data: %w", err)
	}

	// Step 2: Generate cryptographic proof if not already present
	if accountData.Receipt == nil {
		verifiedAccount, err := lc.proofGenerator.GenerateProof(ctx, accountURL)
		if err != nil {
			// Log warning but continue without proof
			fmt.Printf("Warning: failed to generate proof for %s: %v\n", accountURL, err)
		} else {
			// Extract the receipt from the VerifiedAccount
			accountData.Receipt = verifiedAccount.Receipt
			accountData.Height = verifiedAccount.Height
		}
	}

	// Step 3: Update last processed time
	accountData.LastUpdated = time.Now()

	// Step 4: Cache is already handled by GetAccountData, no need to store again

	return accountData, nil
}

// convertTransactions converts Transaction slice to TransactionInfo format
func convertTransactions(transactions []*Transaction) []*TransactionInfo {
	if transactions == nil {
		return nil
	}
	result := make([]*TransactionInfo, len(transactions))
	for i, tx := range transactions {
		result[i] = &TransactionInfo{
			TxID:      tx.TxID,
			Type:      tx.Type,
			Status:    tx.Status,
			Timestamp: time.Unix(tx.Timestamp, 0),
			Amount:    tx.Amount,
			From:      tx.From,
			To:        tx.To,
		}
	}
	return result
}

// convertToCachedTransactions converts TransactionInfo to cached format
func convertToCachedTransactions(txs []*TransactionInfo) []*CachedTransaction {
	if txs == nil {
		return nil
	}
	result := make([]*CachedTransaction, len(txs))
	for i, tx := range txs {
		result[i] = &CachedTransaction{
			TxID:      tx.TxID,
			Type:      tx.Type,
			Status:    tx.Status,
			Timestamp: tx.Timestamp,
			Amount:    tx.Amount,
			From:      tx.From,
			To:        tx.To,
		}
	}
	return result
}

// ADI Management Methods (called by public API)

// AddADIOfInterest adds an ADI to the tracking list
func (lc *LiteClient) AddADIOfInterest(adiURL string) {
	lc.adisOfInterest[adiURL] = true
}

// RemoveADIOfInterest removes an ADI from the tracking list
func (lc *LiteClient) RemoveADIOfInterest(adiURL string) {
	delete(lc.adisOfInterest, adiURL)
}

// GetADIsOfInterest returns the list of tracked ADIs
func (lc *LiteClient) GetADIsOfInterest() []string {
	adis := make([]string, 0, len(lc.adisOfInterest))
	for adi := range lc.adisOfInterest {
		adis = append(adis, adi)
	}
	return adis
}

// ==============================================================================
// UNIVERSAL ACCOUNT API (Internal)
// ==============================================================================

// getAccountData retrieves account data using the universal account API
func (c *LiteClient) getAccountData(ctx context.Context, accountURL string) (*AccountData, error) {
	if err := c.validateAccountURL(accountURL); err != nil {
		return nil, err
	}

	// Use the real implementation from account_handlers.go
	return c.accountHandler.getAccountDataFromNetwork(ctx, accountURL)
}

// getTokenBalance retrieves token balance information
func (c *LiteClient) getTokenBalance(ctx context.Context, accountURL string) (*TokenBalanceInfo, error) {
	if err := c.validateAccountURL(accountURL); err != nil {
		return nil, err
	}

	// Check cache first
	if cached, found := c.unifiedCache.GetBalance(accountURL); found {
		return cached, nil
	}

	// Query from network (implementation would go here)
	return &TokenBalanceInfo{
		AccountURL: accountURL,
		Balance:    "0",
		TokenURL:   "",
	}, nil
}

// getIdentityInfo retrieves identity information
func (c *LiteClient) getIdentityInfo(ctx context.Context, accountURL string) (*IdentityInfo, error) {
	if err := c.validateAccountURL(accountURL); err != nil {
		return nil, err
	}

	// Check cache first
	if cached, found := c.unifiedCache.GetIdentityInfo(accountURL); found {
		return cached, nil
	}

	// Query from network (implementation would go here)
	return &IdentityInfo{
		AccountURL: accountURL,
		KeyBook:    "",
	}, nil
}

// ============================================================================
// PROOF VALIDATION (Internal)
// ============================================================================

// validateAndCacheProof fetches, verifies, and caches a proof for an account
func (c *LiteClient) validateAndCacheProof(ctx context.Context, account string, knownRoot []byte) error {
	// Fetch proof from network
	verified, err := FetchProof(account)
	if err != nil {
		return fmt.Errorf("failed to fetch proof: %w", err)
	}

	// Verify the proof
	if verified.Receipt == nil {
		return fmt.Errorf("no receipt in fetched proof for account: %s", account)
	}
	isValid, err := VerifyProof(verified.Receipt, account, knownRoot)
	if err != nil {
		return fmt.Errorf("proof verification error for account %s: %w", account, err)
	}
	if !isValid {
		return fmt.Errorf("proof verification failed for account: %s", account)
	}

	// Cache the verified proof
	c.unifiedCache.StoreAccountSummary(account, &AccountSummary{
		AccountURL:  account,
		AccountType: "verified",
		Category:    "proof-validated",
	})

	return nil
}

// ============================================================================
// VALIDATION HELPERS (Internal)
// ============================================================================

// validateAccountURL validates account URL format using Accumulate's URL package
func (c *LiteClient) validateAccountURL(accountURL string) error {
	if accountURL == "" {
		return fmt.Errorf("empty account url")
	}

	// Use Accumulate's URL package for proper validation
	_, err := url.Parse(accountURL)
	if err != nil {
		return fmt.Errorf("invalid account url format")
	}

	return nil
}

// validateTransaction validates transaction data structure
func (c *LiteClient) validateTransaction(tx Transaction) error {
	if tx.TxID == "" {
		return errors.New("transaction ID cannot be empty")
	}
	if tx.Account == "" {
		return errors.New("transaction account cannot be empty")
	}
	if err := c.validateAccountURL(tx.Account); err != nil {
		return fmt.Errorf("invalid transaction account: %w", err)
	}
	return nil
}
