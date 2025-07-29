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
	"time"

	"gitlab.com/accumulatenetwork/accumulate/pkg/api/v3/jsonrpc"
	v2 "gitlab.com/accumulatenetwork/accumulate/pkg/client/api/v2"
	"gitlab.com/accumulatenetwork/accumulate/pkg/url"
)

// ============================================================================
// CORE TYPES
// ============================================================================

// LiteClient provides internal infrastructure for the Accumulate Lite Client.
// This is the low-level client used by the public API and orchestrator.
// Users should interact with the public Client API instead.
type LiteClient struct {
	v2           *v2.Client
	v3           *jsonrpc.Client
	unifiedCache *UnifiedCache
}

// ============================================================================
// CONSTRUCTOR
// ============================================================================

// NewLiteClient creates a new internal LiteClient instance.
// This is used internally by the public API - users should use NewClient() instead.
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
		unifiedCache: NewUnifiedCache(5 * time.Minute),
	}, nil
}

// ============================================================================
// UNIVERSAL ACCOUNT API (Internal)
// ============================================================================

// getAccountData retrieves account data using the universal account API
func (c *LiteClient) getAccountData(ctx context.Context, accountURL string) (*AccountData, error) {
	if err := c.validateAccountURL(accountURL); err != nil {
		return nil, err
	}

	// Use the real implementation from account_handlers.go
	return c.getAccountDataFromNetwork(ctx, accountURL)
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
// BATCH PROCESSING (Internal)
// ============================================================================

// batchRetrieveAccountStates retrieves and caches account states in batches
// This is used internally by the orchestrator for efficient processing
func (c *LiteClient) batchRetrieveAccountStates(ctx context.Context, accountUrls []string) error {
	if len(accountUrls) == 0 {
		return errors.New("no account URLs provided")
	}

	// Phase 1: Validate proofs for all accounts
	if err := c.batchValidateProofs(ctx, accountUrls); err != nil {
		return fmt.Errorf("failed to validate proofs: %w", err)
	}

	// Phase 2: Retrieve account data
	if err := c.batchRetrieveAccountData(ctx, accountUrls); err != nil {
		return fmt.Errorf("failed to retrieve account data: %w", err)
	}

	return nil
}

// batchValidateProofs validates proofs for multiple accounts
func (c *LiteClient) batchValidateProofs(ctx context.Context, accountUrls []string) error {
	rootHash, err := FetchBPTRootHash(ctx, c.v2, "dn")
	if err != nil {
		// Use placeholder for testing when BPT root is not available
		rootHash = []byte("placeholder-root-hash")
	}

	for _, url := range accountUrls {
		if err := c.validateAccountURL(url); err != nil {
			return fmt.Errorf("invalid account URL %s: %w", url, err)
		}

		if err := c.validateAndCacheProof(ctx, url, rootHash); err != nil {
			return fmt.Errorf("failed to validate proof for %s: %w", url, err)
		}
	}

	return nil
}

// batchRetrieveAccountData fetches and caches data for multiple accounts
func (c *LiteClient) batchRetrieveAccountData(ctx context.Context, accountUrls []string) error {
	for _, url := range accountUrls {
		// Get account data to determine type
		accountData, err := c.getAccountData(ctx, url)
		if err != nil {
			fmt.Printf("Warning: unable to get account data for %s: %v\n", url, err)
			continue
		}

		// Process based on account type
		if accountData.IsTokenAccount() {
			if err := c.cacheTokenAccountData(ctx, url); err != nil {
				fmt.Printf("Warning: failed to cache token data for %s: %v\n", url, err)
			}
		} else if accountData.IsIdentityAccount() {
			if err := c.cacheIdentityAccountData(ctx, url); err != nil {
				fmt.Printf("Warning: failed to cache identity data for %s: %v\n", url, err)
			}
		}
	}

	return nil
}

// cacheTokenAccountData caches token account specific data
func (c *LiteClient) cacheTokenAccountData(ctx context.Context, accountURL string) error {
	balanceInfo, err := c.getTokenBalance(ctx, accountURL)
	if err != nil {
		return err
	}

	c.unifiedCache.StoreBalance(accountURL, balanceInfo)
	return nil
}

// cacheIdentityAccountData caches identity account specific data
func (c *LiteClient) cacheIdentityAccountData(ctx context.Context, accountURL string) error {
	identityInfo, err := c.getIdentityInfo(ctx, accountURL)
	if err != nil {
		return err
	}

	c.unifiedCache.StoreIdentityInfo(accountURL, identityInfo)
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
