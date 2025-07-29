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
	impl   *LiteClient
	orch   *ADIOrchestrator
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

	// Create internal lite client
	impl, err := NewLiteClient(config.Network.ServerURL)
	if err != nil {
		return nil, fmt.Errorf("failed to create lite client: %w", err)
	}

	// Update cache settings based on config
	impl.unifiedCache.defaultTTL = config.Cache.DefaultTTL

	// Create ADI orchestrator
	orch, err := NewADIOrchestrator(impl)
	if err != nil {
		return nil, fmt.Errorf("failed to create ADI orchestrator: %w", err)
	}

	return &Client{
		config: config,
		impl:   impl,
		orch:   orch,
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

// Close releases all resources used by the client
func (c *Client) Close() error {
	if c.orch != nil {
		return c.orch.Close()
	}
	return nil
}

// GetADI retrieves complete information about an ADI and all its accounts
// This is the main entry point for the simplified API
func (c *Client) GetADI(ctx context.Context, adiURL string) (*ADIData, error) {
	if adiURL == "" {
		return nil, fmt.Errorf("ADI URL cannot be empty")
	}

	// Check cache first
	if cachedData := c.getCachedADI(adiURL); cachedData != nil {
		return cachedData, nil
	}

	// Use orchestrator to get ADI data (conversion handled internally)
	return c.orch.GetADIData(ctx, adiURL)
}

// GetCachedADIs returns a list of ADI URLs that have cached data
func (c *Client) GetCachedADIs() []string {
	return c.impl.unifiedCache.GetCachedADIs()
}

// ClearCache clears all cached data
func (c *Client) ClearCache() error {
	c.impl.unifiedCache.Clear()
	return nil
}

// AddADIOfInterest adds an ADI to the list of ADIs this client cares about
// This enables automatic caching and background updates for the ADI
func (c *Client) AddADIOfInterest(adiURL string) error {
	if adiURL == "" {
		return fmt.Errorf("ADI URL cannot be empty")
	}
	return c.orch.AddADIOfInterest(adiURL)
}

// RemoveADIOfInterest removes an ADI from the list of ADIs this client cares about
// This will also prune all cached data for that ADI
func (c *Client) RemoveADIOfInterest(adiURL string) error {
	if adiURL == "" {
		return fmt.Errorf("ADI URL cannot be empty")
	}
	return c.orch.RemoveADIOfInterest(adiURL)
}

// GetADIsOfInterest returns the list of ADIs this client is currently tracking
func (c *Client) GetADIsOfInterest() []string {
	return c.orch.GetADIsOfInterest()
}

// getCachedADI checks if we have valid cached data for an ADI
func (c *Client) getCachedADI(adiURL string) *ADIData {
	// Check if we have cached account data for this ADI
	accounts := c.impl.unifiedCache.GetADIAccounts(adiURL)
	if len(accounts) == 0 {
		return nil
	}

	// Check if the data is still fresh
	oldest := time.Now()
	for _, account := range accounts {
		if account.LastUpdated.Before(oldest) {
			oldest = account.LastUpdated
		}
	}

	// If oldest data is beyond TTL, consider it stale
	if time.Since(oldest) > c.config.Cache.DefaultTTL {
		return nil
	}

	// Convert cached accounts to simplified ADI data
	simplifiedAccounts := make([]*SimpleAccountData, len(accounts))
	for i, account := range accounts {
		// Get transactions for this account
		txs, _ := c.impl.unifiedCache.GetTransactions(account.URL)
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

// Simplified Data Structures for Public API

// ADIData represents complete information about an ADI and all its accounts
type ADIData struct {
	URL         string               `json:"url"`
	Accounts    []*SimpleAccountData `json:"accounts"`
	LastUpdated time.Time            `json:"lastUpdated"`
	FromCache   bool                 `json:"fromCache"`
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
