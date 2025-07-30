// Copyright 2025 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package liteclient

import (
	"context"
	"fmt"
	"time"
)

// Client is the simplified public interface for the Accumulate Lite Client.
// Users specify an ADI and get all data - proofs, caching, and validation are handled automatically.
type Client struct {
	config *Config
	core   *LiteClient
}

// NewClient creates a new lite client with the provided configuration.
// If config is nil, DefaultConfig() will be used.
func NewClient(config *Config) (*Client, error) {
	if config == nil {
		config = DefaultConfig()
	}

	if err := config.Validate(); err != nil {
		return nil, fmt.Errorf("invalid configuration: %w", err)
	}

	// Create internal lite client with the unified architecture
	core, err := NewLiteClient(config.Network.ServerURL)
	if err != nil {
		return nil, fmt.Errorf("failed to create lite client: %w", err)
	}

	// Update cache settings based on config
	core.unifiedCache.defaultTTL = config.Cache.DefaultTTL

	return &Client{
		config: config,
		core:   core,
	}, nil
}

// NewMainnetClient creates a client configured for Accumulate mainnet
func NewMainnetClient() (*Client, error) {
	return NewClient(DefaultConfig())
}

// NewTestnetClient creates a client configured for Accumulate testnet
func NewTestnetClient() (*Client, error) {
	return NewClient(TestnetConfig())
}

// NewDevnetClient creates a client configured for local development
func NewDevnetClient() (*Client, error) {
	return NewClient(DevnetConfig())
}

// GetADI retrieves complete information about an ADI and all its accounts
// This is the main entry point for the simplified API
// Automatically handles cache freshness verification and receipt construction/verification
func (c *Client) GetADI(ctx context.Context, adiURL string) (*ADIData, error) {
	if adiURL == "" {
		return nil, fmt.Errorf("ADI URL cannot be empty")
	}

	// Check cache first with freshness verification
	if cachedData := c.getCachedADI(adiURL); cachedData != nil && c.isCacheDataFresh(cachedData) {
		return cachedData, nil
	}

	// Track this ADI as one we're interested in
	c.core.AddADIOfInterest(adiURL)

	// Process the ADI using the unified core client
	verifiedAccounts, err := c.core.ProcessADI(ctx, adiURL)
	if err != nil {
		return nil, fmt.Errorf("failed to process ADI: %w", err)
	}

	// Convert to simplified public API format
	return c.convertToADIData(adiURL, verifiedAccounts), nil
}

// GetCachedADIs returns a list of ADI URLs that have cached data
func (c *Client) GetCachedADIs() []string {
	return c.core.unifiedCache.GetCachedADIs()
}

// GetCacheMetadata returns cache metadata for freshness verification
func (c *Client) GetCacheMetadata(adiURL string) (*CacheMetadata, error) {
	if adiURL == "" {
		return nil, fmt.Errorf("ADI URL cannot be empty")
	}

	accounts := c.core.unifiedCache.GetADIAccounts(adiURL)
	if len(accounts) == 0 {
		return nil, fmt.Errorf("no cached data for ADI: %s", adiURL)
	}

	// Find oldest and newest update times
	oldest := time.Now()
	newest := time.Time{}
	for _, account := range accounts {
		if account.LastUpdated.Before(oldest) {
			oldest = account.LastUpdated
		}
		if account.LastUpdated.After(newest) {
			newest = account.LastUpdated
		}
	}

	return &CacheMetadata{
		ADIURL:       adiURL,
		AccountCount: len(accounts),
		OldestUpdate: oldest,
		NewestUpdate: newest,
		IsFresh:      time.Since(oldest) <= c.config.Cache.DefaultTTL,
		TTL:          c.config.Cache.DefaultTTL,
	}, nil
}

// convertToADIData converts internal AccountData to public ADIData format
func (c *Client) convertToADIData(adiURL string, verifiedAccounts []*AccountData) *ADIData {
	// Convert verified accounts to simplified format
	simplifiedAccounts := make([]*SimpleAccountData, len(verifiedAccounts))
	oldest := time.Now()

	for i, account := range verifiedAccounts {
		if account.LastUpdated.Before(oldest) {
			oldest = account.LastUpdated
		}

		// Convert transactions to simplified format
		simplifiedTxs := make([]*SimpleTransaction, len(account.Transactions))
		for j, tx := range account.Transactions {
			simplifiedTxs[j] = &SimpleTransaction{
				TxID:      tx.TxID,
				Type:      tx.Type,
				Status:    tx.Status,
				Timestamp: time.Unix(tx.Timestamp, 0),
				Amount:    tx.Amount,
				From:      tx.From,
				To:        tx.To,
			}
		}

		simplifiedAccounts[i] = &SimpleAccountData{
			URL:          account.URL,
			Type:         account.Type.String(),
			Balance:      account.Balance,
			Transactions: simplifiedTxs,
		}
	}

	return &ADIData{
		URL:         adiURL,
		Accounts:    simplifiedAccounts,
		LastUpdated: oldest,
		FromCache:   false, // This is fresh data from processing
	}
}

// ClearCache clears all cached data
func (c *Client) ClearCache() error {
	c.core.unifiedCache.Clear()
	return nil
}

// AddADIOfInterest adds an ADI to the list of ADIs this client cares about
// This enables automatic caching and background updates for the ADI
func (c *Client) AddADIOfInterest(adiURL string) error {
	c.core.AddADIOfInterest(adiURL)
	return nil
}

// RemoveADIOfInterest removes an ADI from the list of ADIs this client cares about
// This will also prune all cached data for that ADI
func (c *Client) RemoveADIOfInterest(adiURL string) error {
	c.core.RemoveADIOfInterest(adiURL)
	return c.PruneADI(adiURL)
}

// PruneADI removes all cached data for a specific ADI
func (c *Client) PruneADI(adiURL string) error {
	if adiURL == "" {
		return fmt.Errorf("ADI URL cannot be empty")
	}

	// Remove all accounts under this ADI from cache
	accounts := c.core.unifiedCache.GetADIAccounts(adiURL)
	for _, account := range accounts {
		c.core.unifiedCache.RemoveAccount(account.URL)
	}

	return nil
}

// PruneAccount removes cached data for a specific account under an ADI
func (c *Client) PruneAccount(accountURL string) error {
	if accountURL == "" {
		return fmt.Errorf("account URL cannot be empty")
	}

	c.core.unifiedCache.RemoveAccount(accountURL)
	return nil
}

// PruneStaleData removes cached data older than the specified duration
func (c *Client) PruneStaleData(olderThan time.Duration) error {
	cachedADIs := c.core.unifiedCache.GetCachedADIs()
	for _, adiURL := range cachedADIs {
		accounts := c.core.unifiedCache.GetADIAccounts(adiURL)
		for _, account := range accounts {
			if time.Since(account.LastUpdated) > olderThan {
				c.core.unifiedCache.RemoveAccount(account.URL)
			}
		}
	}
	return nil
}

// GetADIsOfInterest returns the list of ADIs this client is currently tracking
func (c *Client) GetADIsOfInterest() []string {
	return c.core.GetADIsOfInterest()
}

// getCachedADI checks if we have cached data for an ADI (without freshness check)
func (c *Client) getCachedADI(adiURL string) *ADIData {
	// Check if we have cached account data for this ADI
	accounts := c.core.unifiedCache.GetADIAccounts(adiURL)
	if len(accounts) == 0 {
		return nil
	}

	// Find oldest update time for metadata
	oldest := time.Now()
	for _, account := range accounts {
		if account.LastUpdated.Before(oldest) {
			oldest = account.LastUpdated
		}
	}

	// Convert cached accounts to simplified ADI data
	simplifiedAccounts := make([]*SimpleAccountData, len(accounts))
	for i, account := range accounts {
		// Get transactions for this account
		txs, _ := c.core.unifiedCache.GetTransactions(account.URL)
		simplifiedTxs := make([]*SimpleTransaction, len(txs))
		for j, tx := range txs {
			simplifiedTxs[j] = &SimpleTransaction{
				TxID:      tx.TxID,
				Type:      tx.Type,
				Status:    tx.Status,
				Timestamp: tx.Timestamp, // tx.Timestamp is already time.Time
				Amount:    tx.Amount,
				From:      tx.From,
				To:        tx.To,
			}
		}

		simplifiedAccounts[i] = &SimpleAccountData{
			URL:          account.URL,
			Type:         account.Type.String(),
			Balance:      account.Balance,
			Transactions: simplifiedTxs,
		}
	}

	return &ADIData{
		URL:         adiURL,
		Accounts:    simplifiedAccounts,
		LastUpdated: oldest,
		FromCache:   true,
	}
}

// isCacheDataFresh verifies if cached data meets freshness requirements
func (c *Client) isCacheDataFresh(data *ADIData) bool {
	if data == nil {
		return false
	}
	// Check if data is within TTL
	return time.Since(data.LastUpdated) <= c.config.Cache.DefaultTTL
}

// VerifyReceipt manually verifies a receipt for transparency
// This exposes the receipt verification process to users
func (c *Client) VerifyReceipt(ctx context.Context, accountURL string) (*ReceiptVerificationResult, error) {
	if accountURL == "" {
		return nil, fmt.Errorf("account URL cannot be empty")
	}

	// Use the existing FetchProof function to construct and verify receipt
	verifiedAccount, err := FetchProof(accountURL)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch proof: %w", err)
	}

	// Verify the receipt if it exists
	isValid := false
	if verifiedAccount.Receipt != nil {
		// Use receipt's built-in validation method
		isValid = verifiedAccount.Receipt.Validate(nil)
	}

	// Extract merkle root from receipt if available
	merkleRoot := ""
	if verifiedAccount.Receipt != nil && verifiedAccount.Receipt.Anchor != nil {
		merkleRoot = fmt.Sprintf("%x", verifiedAccount.Receipt.Anchor)
	}

	return &ReceiptVerificationResult{
		AccountURL:    accountURL,
		ReceiptValid:  isValid,
		MerkleRoot:    merkleRoot,
		BlockHeight:   uint64(verifiedAccount.Height),
		VerifiedAt:    time.Now(),
		ReceiptExists: verifiedAccount.Receipt != nil,
	}, nil
}

// Simplified Data Structures for Public API

// ADIData represents complete information about an ADI and all its accounts
type ADIData struct {
	URL         string               `json:"url"`
	Accounts    []*SimpleAccountData `json:"accounts"`
	LastUpdated time.Time            `json:"lastUpdated"`
	FromCache   bool                 `json:"fromCache"`
}

// CacheMetadata provides information about cached data freshness
type CacheMetadata struct {
	ADIURL       string        `json:"adiUrl"`
	AccountCount int           `json:"accountCount"`
	OldestUpdate time.Time     `json:"oldestUpdate"`
	NewestUpdate time.Time     `json:"newestUpdate"`
	IsFresh      bool          `json:"isFresh"`
	TTL          time.Duration `json:"ttl"`
}

// ReceiptVerificationResult contains the result of receipt verification
type ReceiptVerificationResult struct {
	AccountURL    string    `json:"accountUrl"`
	ReceiptValid  bool      `json:"receiptValid"`
	MerkleRoot    string    `json:"merkleRoot"`
	BlockHeight   uint64    `json:"blockHeight"`
	VerifiedAt    time.Time `json:"verifiedAt"`
	ReceiptExists bool      `json:"receiptExists"`
}

// SimpleAccountData represents simplified account information for public API
type SimpleAccountData struct {
	URL          string               `json:"url"`
	Type         string               `json:"type"`
	Balance      string               `json:"balance"`
	Transactions []*SimpleTransaction `json:"transactions"`
}

// SimpleTransaction represents simplified transaction information for public API
type SimpleTransaction struct {
	TxID      string    `json:"txId"`
	Type      string    `json:"type"`
	Status    string    `json:"status"`
	Timestamp time.Time `json:"timestamp"`
	Amount    string    `json:"amount"`
	From      string    `json:"from"`
	To        string    `json:"to"`
}
