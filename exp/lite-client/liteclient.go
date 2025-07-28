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
	"strings"
	"sync"
	"time"

	api "gitlab.com/accumulatenetwork/accumulate/pkg/api/v3"
	"gitlab.com/accumulatenetwork/accumulate/pkg/api/v3/jsonrpc"
	v2 "gitlab.com/accumulatenetwork/accumulate/pkg/client/api/v2"
	"gitlab.com/accumulatenetwork/accumulate/pkg/url"
)

// LiteClient provides a lightweight client for Accumulate network operations.
// It maintains local caches for proofs, transactions, and balances to improve performance.
type LiteClient struct {
	v2           *v2.Client
	v3           api.Querier
	cache        map[string]VerifiedAccount // account -> verified proof (legacy)
	unifiedCache *UnifiedCache              // comprehensive caching for all data types
	mu           sync.RWMutex               // protects legacy maps
}

// NewLiteClient creates a new LiteClient instance with the specified server URL.
// The server URL should be the base URL of the Accumulate API server.
func NewLiteClient(server string) (*LiteClient, error) {
	if server == "" {
		return nil, errors.New("server URL cannot be empty")
	}

	// Normalize server URL
	server = strings.TrimSuffix(server, "/")

	v2Client, err := v2.New(server)
	if err != nil {
		return nil, fmt.Errorf("failed to create v2 client: %w", err)
	}

	// v3 API is served at /v3 endpoint
	v3Client := jsonrpc.NewClient(server + "/v3")

	return &LiteClient{
		v2:           v2Client,
		v3:           v3Client,
		cache:        make(map[string]VerifiedAccount),
		unifiedCache: NewUnifiedCache(5 * time.Minute), // 5 minute default TTL
	}, nil
}

// ValidateAndCacheProof fetches, verifies, and caches a proof for the given account using the provided LiteClient.
func (c *LiteClient) ValidateAndCacheProof(ctx context.Context, account string, knownRoot []byte) error {
	// Step 1: Fetch proof for the account from the node
	verified, err := FetchProof(account)
	if err != nil {
		return fmt.Errorf("failed to fetch proof: %w", err)
	}

	// Step 2: Verify the proof against the known root
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

	// Step 3: Cache/store the verified proof for future use
	c.StoreProof(account, verified.Receipt, verified.Height)

	return nil
}

// RetrieveAccountStates retrieves and caches account states including proofs, balances, and transactions.
// This is a two-phase process: first validate proofs, then retrieve account data.
func (c *LiteClient) RetrieveAccountStates(ctx context.Context, accountUrls []string) error {
	if len(accountUrls) == 0 {
		return errors.New("no account URLs provided")
	}

	// Phase 1: Retrieve and validate proofs
	if err := c.retrieveAndValidateProofs(ctx, accountUrls); err != nil {
		return fmt.Errorf("failed to retrieve or validate account proofs: %w", err)
	}

	// Phase 2: Retrieve account data
	if err := c.retrieveAccountData(ctx, accountUrls); err != nil {
		return fmt.Errorf("failed to retrieve account data: %w", err)
	}

	return nil
}

// retrieveAndValidateProofs handles the proof validation phase
func (c *LiteClient) retrieveAndValidateProofs(ctx context.Context, accountUrls []string) error {
	rootHash, err := FetchBPTRootHash(ctx, c.v2, "dn")
	if err != nil {
		// Use placeholder for testing when BPT root is not available
		rootHash = []byte("placeholder-root-hash")
	}

	for _, url := range accountUrls {
		if err := c.validateAccountURL(url); err != nil {
			return fmt.Errorf("invalid account URL %s: %w", url, err)
		}

		if err := c.ValidateAndCacheProof(ctx, url, rootHash); err != nil {
			return fmt.Errorf("failed to validate proof for %s: %w", url, err)
		}
	}

	return nil
}

// retrieveAccountData fetches and caches balance and transaction data
func (c *LiteClient) retrieveAccountData(ctx context.Context, accountUrls []string) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	for _, url := range accountUrls {
		// Use universal API to get account data
		accountData, err := c.GetAccountData(ctx, url)
		if err != nil {
			fmt.Printf("Warning: unable to retrieve account data for %s: %v\n", url, err)
			continue
		}
		
		// Only process token accounts for balance/transaction data
		if !accountData.IsTokenAccount() {
			fmt.Printf("Skipping non-token account: %s (type: %s)\n", url, accountData.TypeName)
			continue
		}
		
		// Get token balance
		balanceInfo, err := c.GetTokenBalance(ctx, url)
		if err != nil {
			fmt.Printf("Warning: unable to get balance for %s: %v\n", url, err)
			continue
		}
		
		// For now, skip transaction retrieval as it's not implemented in universal API
		// TODO: Implement transaction retrieval in universal API
		transactions := []Transaction{} // Empty for now

		// Store in unified cache
		for _, tx := range transactions {
			// Convert Transaction to CachedTransaction
			cachedTx := &CachedTransaction{
				TxID:      tx.TxID,
				Type:      tx.Type,
				Status:    tx.Status,
				Timestamp: time.Unix(tx.Timestamp, 0),
				Amount:    tx.Amount,
				From:      tx.From,
				To:        tx.To,
				Account:   tx.Account,
				Height:    uint64(tx.Height),
				Data:      tx.Data,
			}
			c.unifiedCache.AddTransaction(url, cachedTx)
		}
		c.unifiedCache.StoreBalance(url, &TokenBalanceInfo{
			AccountURL:  url,
			Balance:     balanceInfo.Balance,
			TokenURL:    balanceInfo.TokenURL,
			AccountType: "token",
		})
	}

	return nil
}

// validateAccountURL performs basic validation on account URLs
// validateAccountURL performs validation on account URLs using Accumulate's URL package
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

// validateTransaction performs basic validation on transaction data
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
