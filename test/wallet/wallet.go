// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package wallet

import (
	"crypto/ed25519"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"gitlab.com/accumulatenetwork/accumulate/pkg/url"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

// Account represents a test account with its keys.
type Account struct {
	URL        string `json:"url"`
	PublicKey  string `json:"publicKey"`  // hex encoded
	PrivateKey string `json:"privateKey"` // hex encoded
	Type       string `json:"type"`       // "lite", "adi-token", "adi-data"
	Index      int    `json:"index"`
}

// Wallet stores test accounts and their keys.
type Wallet struct {
	Funder   *Account   `json:"funder"`
	Accounts []*Account `json:"accounts"`
	path     string
}

// New creates a new empty wallet.
func New(path string) *Wallet {
	return &Wallet{
		path:     path,
		Accounts: make([]*Account, 0),
	}
}

// Load loads a wallet from disk.
func Load(path string) (*Wallet, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read wallet file: %w", err)
	}

	w := &Wallet{path: path}
	if err := json.Unmarshal(data, w); err != nil {
		return nil, fmt.Errorf("unmarshal wallet: %w", err)
	}

	return w, nil
}

// Save saves the wallet to disk.
func (w *Wallet) Save() error {
	// Create directory if it doesn't exist
	dir := filepath.Dir(w.path)
	if err := os.MkdirAll(dir, 0700); err != nil {
		return fmt.Errorf("create wallet directory: %w", err)
	}

	data, err := json.MarshalIndent(w, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal wallet: %w", err)
	}

	if err := os.WriteFile(w.path, data, 0600); err != nil {
		return fmt.Errorf("write wallet file: %w", err)
	}

	return nil
}

// GenerateFunder generates a new funder account with the given URL.
func (w *Wallet) GenerateFunder(funderURL string) error {
	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		return fmt.Errorf("generate funder key: %w", err)
	}

	w.Funder = &Account{
		URL:        funderURL,
		PublicKey:  hex.EncodeToString(pub),
		PrivateKey: hex.EncodeToString(priv),
		Type:       "funder",
		Index:      -1,
	}

	return nil
}

// GenerateAccounts generates test accounts.
func (w *Wallet) GenerateAccounts(numLite, numADIToken, numADIData int) error {
	index := 0

	// Generate lite accounts
	for i := 0; i < numLite; i++ {
		pub, priv, err := ed25519.GenerateKey(rand.Reader)
		if err != nil {
			return fmt.Errorf("generate lite account key: %w", err)
		}

		liteURL, err := protocol.LiteTokenAddress(pub, protocol.ACME, protocol.SignatureTypeED25519)
		if err != nil {
			return fmt.Errorf("create lite address: %w", err)
		}

		w.Accounts = append(w.Accounts, &Account{
			URL:        liteURL.String(),
			PublicKey:  hex.EncodeToString(pub),
			PrivateKey: hex.EncodeToString(priv),
			Type:       "lite",
			Index:      index,
		})
		index++
	}

	// Generate ADI token accounts
	for i := 0; i < numADIToken; i++ {
		pub, priv, err := ed25519.GenerateKey(rand.Reader)
		if err != nil {
			return fmt.Errorf("generate ADI token account key: %w", err)
		}

		// ADI format: acc://test-a00000001.acme/tokens
		adiURL := protocol.AccountUrl(fmt.Sprintf("test-a%08d", i+1)).JoinPath("tokens")

		w.Accounts = append(w.Accounts, &Account{
			URL:        adiURL.String(),
			PublicKey:  hex.EncodeToString(pub),
			PrivateKey: hex.EncodeToString(priv),
			Type:       "adi-token",
			Index:      index,
		})
		index++
	}

	// Generate ADI data accounts
	for i := 0; i < numADIData; i++ {
		pub, priv, err := ed25519.GenerateKey(rand.Reader)
		if err != nil {
			return fmt.Errorf("generate ADI data account key: %w", err)
		}

		// ADI format: acc://test-d00000001.acme/data
		adiURL := protocol.AccountUrl(fmt.Sprintf("test-d%08d", i+1)).JoinPath("data")

		w.Accounts = append(w.Accounts, &Account{
			URL:        adiURL.String(),
			PublicKey:  hex.EncodeToString(pub),
			PrivateKey: hex.EncodeToString(priv),
			Type:       "adi-data",
			Index:      index,
		})
		index++
	}

	return nil
}

// GetAccount retrieves an account by index.
func (w *Wallet) GetAccount(index int) (*Account, error) {
	if index < 0 || index >= len(w.Accounts) {
		return nil, fmt.Errorf("account index %d out of range [0, %d)", index, len(w.Accounts))
	}
	return w.Accounts[index], nil
}

// GetAccountByURL retrieves an account by URL.
func (w *Wallet) GetAccountByURL(urlStr string) (*Account, error) {
	for _, acct := range w.Accounts {
		if acct.URL == urlStr {
			return acct, nil
		}
	}
	return nil, fmt.Errorf("account not found: %s", urlStr)
}

// GetAccountsByType retrieves all accounts of a given type.
func (w *Wallet) GetAccountsByType(accountType string) []*Account {
	var accounts []*Account
	for _, acct := range w.Accounts {
		if acct.Type == accountType {
			accounts = append(accounts, acct)
		}
	}
	return accounts
}

// GetPrivateKey returns the decoded private key for an account.
func (a *Account) GetPrivateKey() (ed25519.PrivateKey, error) {
	key, err := hex.DecodeString(a.PrivateKey)
	if err != nil {
		return nil, fmt.Errorf("decode private key: %w", err)
	}
	return ed25519.PrivateKey(key), nil
}

// GetPublicKey returns the decoded public key for an account.
func (a *Account) GetPublicKey() (ed25519.PublicKey, error) {
	key, err := hex.DecodeString(a.PublicKey)
	if err != nil {
		return nil, fmt.Errorf("decode public key: %w", err)
	}
	return ed25519.PublicKey(key), nil
}

// GetURL returns the parsed URL for an account.
func (a *Account) GetURL() (*url.URL, error) {
	return url.Parse(a.URL)
}

// Count returns the total number of accounts.
func (w *Wallet) Count() int {
	return len(w.Accounts)
}

// CountByType returns the number of accounts of a given type.
func (w *Wallet) CountByType(accountType string) int {
	count := 0
	for _, acct := range w.Accounts {
		if acct.Type == accountType {
			count++
		}
	}
	return count
}
