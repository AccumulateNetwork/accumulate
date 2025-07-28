// Copyright 2025 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package liteclient

import (
	"sync"
	"time"
)

// UnifiedCache provides comprehensive caching for all account data types
// This replaces the limited transaction-only caching with support for all Accumulate data
type UnifiedCache struct {
	mu               sync.RWMutex
	accountData      map[string]*CachedAccountData
	transactions     map[string][]*CachedTransaction
	balances         map[string]*CachedBalance
	identityInfo     map[string]*CachedIdentityInfo
	dataAccountInfo  map[string]*CachedDataAccountInfo
	accountSummaries map[string]*CachedAccountSummary
	defaultTTL       time.Duration
}

// CachedAccountData represents cached account data with metadata
type CachedAccountData struct {
	Data      *AccountData `json:"data"`
	CachedAt  time.Time    `json:"cachedAt"`
	ExpiresAt time.Time    `json:"expiresAt"`
	URL       string       `json:"url"`
}

// CachedTransaction represents cached transaction data
type CachedTransaction struct {
	TxID      string      `json:"txId"`
	Type      string      `json:"type"`
	Status    string      `json:"status"`
	Timestamp time.Time   `json:"timestamp"`
	Amount    string      `json:"amount"`
	From      string      `json:"from"`
	To        string      `json:"to"`
	Account   string      `json:"account"`
	Height    uint64      `json:"height"`
	Data      interface{} `json:"data"`
	CachedAt  time.Time   `json:"cachedAt"`
	ExpiresAt time.Time   `json:"expiresAt"`
}

// CachedBalance represents cached balance information
type CachedBalance struct {
	Data      *TokenBalanceInfo `json:"data"`
	CachedAt  time.Time         `json:"cachedAt"`
	ExpiresAt time.Time         `json:"expiresAt"`
	URL       string            `json:"url"`
}

// CachedIdentityInfo represents cached identity information
type CachedIdentityInfo struct {
	Data      *IdentityInfo `json:"data"`
	CachedAt  time.Time     `json:"cachedAt"`
	ExpiresAt time.Time     `json:"expiresAt"`
	URL       string        `json:"url"`
}

// CachedDataAccountInfo represents cached data account information
type CachedDataAccountInfo struct {
	Data      *DataAccountInfo `json:"data"`
	CachedAt  time.Time        `json:"cachedAt"`
	ExpiresAt time.Time        `json:"expiresAt"`
	URL       string           `json:"url"`
}

// CachedAccountSummary represents cached account summary
type CachedAccountSummary struct {
	Data      *AccountSummary `json:"data"`
	CachedAt  time.Time       `json:"cachedAt"`
	ExpiresAt time.Time       `json:"expiresAt"`
	URL       string          `json:"url"`
}

// NewUnifiedCache creates a new unified cache with default TTL
func NewUnifiedCache(defaultTTL time.Duration) *UnifiedCache {
	if defaultTTL == 0 {
		defaultTTL = 5 * time.Minute // Default 5 minute TTL
	}

	return &UnifiedCache{
		accountData:      make(map[string]*CachedAccountData),
		transactions:     make(map[string][]*CachedTransaction),
		balances:         make(map[string]*CachedBalance),
		identityInfo:     make(map[string]*CachedIdentityInfo),
		dataAccountInfo:  make(map[string]*CachedDataAccountInfo),
		accountSummaries: make(map[string]*CachedAccountSummary),
		defaultTTL:       defaultTTL,
	}
}

// Account Data Caching Methods

// StoreAccountData stores account data in cache
func (c *UnifiedCache) StoreAccountData(url string, data *AccountData, ttl ...time.Duration) {
	c.mu.Lock()
	defer c.mu.Unlock()

	expiry := c.defaultTTL
	if len(ttl) > 0 {
		expiry = ttl[0]
	}

	c.accountData[url] = &CachedAccountData{
		Data:      data,
		CachedAt:  time.Now(),
		ExpiresAt: time.Now().Add(expiry),
		URL:       url,
	}
}

// GetAccountData retrieves account data from cache
func (c *UnifiedCache) GetAccountData(url string) (*AccountData, bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()

	cached, exists := c.accountData[url]
	if !exists || time.Now().After(cached.ExpiresAt) {
		return nil, false
	}

	return cached.Data, true
}

// Balance Caching Methods

// StoreBalance stores balance information in cache
func (c *UnifiedCache) StoreBalance(url string, balance *TokenBalanceInfo, ttl ...time.Duration) {
	c.mu.Lock()
	defer c.mu.Unlock()

	expiry := c.defaultTTL
	if len(ttl) > 0 {
		expiry = ttl[0]
	}

	c.balances[url] = &CachedBalance{
		Data:      balance,
		CachedAt:  time.Now(),
		ExpiresAt: time.Now().Add(expiry),
		URL:       url,
	}
}

// GetBalance retrieves balance from cache
func (c *UnifiedCache) GetBalance(url string) (*TokenBalanceInfo, bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()

	cached, exists := c.balances[url]
	if !exists || time.Now().After(cached.ExpiresAt) {
		return nil, false
	}

	return cached.Data, true
}

// Identity Info Caching Methods

// StoreIdentityInfo stores identity information in cache
func (c *UnifiedCache) StoreIdentityInfo(url string, info *IdentityInfo, ttl ...time.Duration) {
	c.mu.Lock()
	defer c.mu.Unlock()

	expiry := c.defaultTTL
	if len(ttl) > 0 {
		expiry = ttl[0]
	}

	c.identityInfo[url] = &CachedIdentityInfo{
		Data:      info,
		CachedAt:  time.Now(),
		ExpiresAt: time.Now().Add(expiry),
		URL:       url,
	}
}

// GetIdentityInfo retrieves identity info from cache
func (c *UnifiedCache) GetIdentityInfo(url string) (*IdentityInfo, bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()

	cached, exists := c.identityInfo[url]
	if !exists || time.Now().After(cached.ExpiresAt) {
		return nil, false
	}

	return cached.Data, true
}

// Data Account Info Caching Methods

// StoreDataAccountInfo stores data account information in cache
func (c *UnifiedCache) StoreDataAccountInfo(url string, info *DataAccountInfo, ttl ...time.Duration) {
	c.mu.Lock()
	defer c.mu.Unlock()

	expiry := c.defaultTTL
	if len(ttl) > 0 {
		expiry = ttl[0]
	}

	c.dataAccountInfo[url] = &CachedDataAccountInfo{
		Data:      info,
		CachedAt:  time.Now(),
		ExpiresAt: time.Now().Add(expiry),
		URL:       url,
	}
}

// GetDataAccountInfo retrieves data account info from cache
func (c *UnifiedCache) GetDataAccountInfo(url string) (*DataAccountInfo, bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()

	cached, exists := c.dataAccountInfo[url]
	if !exists || time.Now().After(cached.ExpiresAt) {
		return nil, false
	}

	return cached.Data, true
}

// Account Summary Caching Methods

// StoreAccountSummary stores account summary in cache
func (c *UnifiedCache) StoreAccountSummary(url string, summary *AccountSummary, ttl ...time.Duration) {
	c.mu.Lock()
	defer c.mu.Unlock()

	expiry := c.defaultTTL
	if len(ttl) > 0 {
		expiry = ttl[0]
	}

	c.accountSummaries[url] = &CachedAccountSummary{
		Data:      summary,
		CachedAt:  time.Now(),
		ExpiresAt: time.Now().Add(expiry),
		URL:       url,
	}
}

// GetAccountSummary retrieves account summary from cache
func (c *UnifiedCache) GetAccountSummary(url string) (*AccountSummary, bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()

	cached, exists := c.accountSummaries[url]
	if !exists || time.Now().After(cached.ExpiresAt) {
		return nil, false
	}

	return cached.Data, true
}

// Transaction Caching Methods

// StoreTransactions stores transactions for an account in cache
func (c *UnifiedCache) StoreTransactions(accountURL string, transactions []*CachedTransaction, ttl ...time.Duration) {
	c.mu.Lock()
	defer c.mu.Unlock()

	expiry := c.defaultTTL
	if len(ttl) > 0 {
		expiry = ttl[0]
	}

	// Set expiry for all transactions
	for _, tx := range transactions {
		tx.CachedAt = time.Now()
		tx.ExpiresAt = time.Now().Add(expiry)
	}

	c.transactions[accountURL] = transactions
}

// GetTransactions retrieves transactions for an account from cache
func (c *UnifiedCache) GetTransactions(accountURL string) ([]*CachedTransaction, bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()

	transactions, exists := c.transactions[accountURL]
	if !exists || len(transactions) == 0 {
		return nil, false
	}

	// Check if any transaction has expired (if one expires, refresh all)
	for _, tx := range transactions {
		if time.Now().After(tx.ExpiresAt) {
			return nil, false
		}
	}

	return transactions, true
}

// AddTransaction adds a single transaction to the cache
func (c *UnifiedCache) AddTransaction(accountURL string, transaction *CachedTransaction, ttl ...time.Duration) {
	c.mu.Lock()
	defer c.mu.Unlock()

	expiry := c.defaultTTL
	if len(ttl) > 0 {
		expiry = ttl[0]
	}

	transaction.CachedAt = time.Now()
	transaction.ExpiresAt = time.Now().Add(expiry)

	if c.transactions[accountURL] == nil {
		c.transactions[accountURL] = make([]*CachedTransaction, 0)
	}

	// Add to the beginning (most recent first)
	c.transactions[accountURL] = append([]*CachedTransaction{transaction}, c.transactions[accountURL]...)
}

// Cache Management Methods

// InvalidateAccount removes all cached data for a specific account
func (c *UnifiedCache) InvalidateAccount(url string) {
	c.mu.Lock()
	defer c.mu.Unlock()

	delete(c.accountData, url)
	delete(c.balances, url)
	delete(c.identityInfo, url)
	delete(c.dataAccountInfo, url)
	delete(c.accountSummaries, url)
	delete(c.transactions, url)
}

// InvalidateAll clears all cached data
func (c *UnifiedCache) InvalidateAll() {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.accountData = make(map[string]*CachedAccountData)
	c.transactions = make(map[string][]*CachedTransaction)
	c.balances = make(map[string]*CachedBalance)
	c.identityInfo = make(map[string]*CachedIdentityInfo)
	c.dataAccountInfo = make(map[string]*CachedDataAccountInfo)
	c.accountSummaries = make(map[string]*CachedAccountSummary)
}

// CleanupExpired removes all expired entries from cache
func (c *UnifiedCache) CleanupExpired() {
	c.mu.Lock()
	defer c.mu.Unlock()

	now := time.Now()

	// Clean account data
	for url, cached := range c.accountData {
		if now.After(cached.ExpiresAt) {
			delete(c.accountData, url)
		}
	}

	// Clean balances
	for url, cached := range c.balances {
		if now.After(cached.ExpiresAt) {
			delete(c.balances, url)
		}
	}

	// Clean identity info
	for url, cached := range c.identityInfo {
		if now.After(cached.ExpiresAt) {
			delete(c.identityInfo, url)
		}
	}

	// Clean data account info
	for url, cached := range c.dataAccountInfo {
		if now.After(cached.ExpiresAt) {
			delete(c.dataAccountInfo, url)
		}
	}

	// Clean account summaries
	for url, cached := range c.accountSummaries {
		if now.After(cached.ExpiresAt) {
			delete(c.accountSummaries, url)
		}
	}

	// Clean transactions
	for url, transactions := range c.transactions {
		validTransactions := make([]*CachedTransaction, 0)
		for _, tx := range transactions {
			if now.Before(tx.ExpiresAt) {
				validTransactions = append(validTransactions, tx)
			}
		}
		if len(validTransactions) == 0 {
			delete(c.transactions, url)
		} else {
			c.transactions[url] = validTransactions
		}
	}
}

// GetCacheStats returns statistics about the cache
func (c *UnifiedCache) GetCacheStats() map[string]interface{} {
	c.mu.RLock()
	defer c.mu.RUnlock()

	totalTransactions := 0
	for _, txs := range c.transactions {
		totalTransactions += len(txs)
	}

	return map[string]interface{}{
		"accountData":         len(c.accountData),
		"balances":            len(c.balances),
		"identityInfo":        len(c.identityInfo),
		"dataAccountInfo":     len(c.dataAccountInfo),
		"accountSummaries":    len(c.accountSummaries),
		"transactionAccounts": len(c.transactions),
		"totalTransactions":   totalTransactions,
		"defaultTTL":          c.defaultTTL.String(),
	}
}

// IsStale checks if data for a URL is stale (expired or missing)
func (c *UnifiedCache) IsStale(url string, dataType string) bool {
	c.mu.RLock()
	defer c.mu.RUnlock()

	now := time.Now()

	switch dataType {
	case "accountData":
		cached, exists := c.accountData[url]
		return !exists || now.After(cached.ExpiresAt)
	case "balance":
		cached, exists := c.balances[url]
		return !exists || now.After(cached.ExpiresAt)
	case "identityInfo":
		cached, exists := c.identityInfo[url]
		return !exists || now.After(cached.ExpiresAt)
	case "dataAccountInfo":
		cached, exists := c.dataAccountInfo[url]
		return !exists || now.After(cached.ExpiresAt)
	case "accountSummary":
		cached, exists := c.accountSummaries[url]
		return !exists || now.After(cached.ExpiresAt)
	case "transactions":
		transactions, exists := c.transactions[url]
		if !exists || len(transactions) == 0 {
			return true
		}
		// Check if any transaction has expired
		for _, tx := range transactions {
			if now.After(tx.ExpiresAt) {
				return true
			}
		}
		return false
	default:
		return true // Unknown data type is considered stale
	}
}
