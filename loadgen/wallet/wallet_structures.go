package wallet

import (
	"crypto/ed25519"
	"crypto/sha256"
	"fmt"
	"sync"
	"time"

	"gitlab.com/accumulatenetwork/accumulate/pkg/url"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

// Wallet represents the main wallet container that tracks what the load generator builds
type Wallet struct {
	mu sync.RWMutex

	// Core tracking maps - these track everything we've created
	accounts      map[url.URL]Account          // URL -> Account interface
	adis          map[url.URL]*ADI             // URL -> ADI
	tokenAccounts map[url.URL]*TokenAccount    // URL -> TokenAccount
	dataAccounts  map[url.URL]*DataAccount     // URL -> DataAccount
	tokenIssuers  map[url.URL]*TokenIssuer     // URL -> TokenIssuer
	keyBooks      map[url.URL]*KeyBook         // URL -> KeyBook
	keyPages      map[url.URL]*KeyPage         // URL -> KeyPage
	keys          map[string]*Key              // Public key hash (hex) -> Key
	liteIdentities map[url.URL]*LiteIdentity   // URL -> LiteIdentity
	liteTokenAccounts map[url.URL]*LiteTokenAccount // URL -> LiteTokenAccount
	liteDataAccounts map[url.URL]*LiteDataAccount   // URL -> LiteDataAccount
	
	// Primary funding account - receives ACME from faucet and distributes to other accounts
	fundingAccount *LiteIdentity
	
	// Funding manager for automatic credit maintenance
	fundingManager *FundingManager
	
	// Seed for deterministic key generation
	seed []byte
	liteAccountCounter int
}

// WalletStats tracks wallet statistics for load generation
type WalletStats struct {
	TotalAccounts      int
	TotalADIs          int
	TotalTokenAccounts int
	TotalDataAccounts  int
	TotalTokenIssuers  int
	TotalLiteAccounts  int
	TotalKeyBooks      int
	TotalKeyPages      int
	TotalKeys          int
}

// NewWallet creates a new wallet instance
func NewWallet() *Wallet {
	return NewWalletWithSeed(nil)
}

// NewWalletWithSeed creates a new wallet instance with a specific seed for deterministic key generation
func NewWalletWithSeed(seed []byte) *Wallet {
	// If no seed provided, use a default seed
	if seed == nil {
		seed = []byte("default-wallet-seed")
	}
	
	return &Wallet{
		accounts:          make(map[url.URL]Account),
		adis:              make(map[url.URL]*ADI),
		tokenAccounts:     make(map[url.URL]*TokenAccount),
		dataAccounts:      make(map[url.URL]*DataAccount),
		tokenIssuers:      make(map[url.URL]*TokenIssuer),
		keyBooks:          make(map[url.URL]*KeyBook),
		keyPages:          make(map[url.URL]*KeyPage),
		keys:              make(map[string]*Key),
		liteIdentities:    make(map[url.URL]*LiteIdentity),
		liteTokenAccounts: make(map[url.URL]*LiteTokenAccount),
		liteDataAccounts:  make(map[url.URL]*LiteDataAccount),
		seed:              seed,
		liteAccountCounter: 0,
	}
}

// Helper methods for URL management

func (w *Wallet) GetAccount(u *url.URL) Account {
	w.mu.RLock()
	defer w.mu.RUnlock()
	if u == nil {
		return nil
	}
	return w.accounts[*u]
}

func (w *Wallet) GetADI(u *url.URL) *ADI {
	w.mu.RLock()
	defer w.mu.RUnlock()
	if u == nil {
		return nil
	}
	return w.adis[*u]
}

// GetADIByName looks up ADI by iterating through all ADIs
func (w *Wallet) GetADIByName(name string) *ADI {
	w.mu.RLock()
	defer w.mu.RUnlock()
	for _, adi := range w.adis {
		if adi.Name == name {
			return adi
		}
	}
	return nil
}

func (w *Wallet) GetKeyBook(u *url.URL) *KeyBook {
	w.mu.RLock()
	defer w.mu.RUnlock()
	if u == nil {
		return nil
	}
	return w.keyBooks[*u]
}

func (w *Wallet) GetKeyPage(u *url.URL) *KeyPage {
	w.mu.RLock()
	defer w.mu.RUnlock()
	if u == nil {
		return nil
	}
	return w.keyPages[*u]
}

func (w *Wallet) GetKey(publicKeyHashHex string) *Key {
	w.mu.RLock()
	defer w.mu.RUnlock()
	return w.keys[publicKeyHashHex]
}

// GetKeyByHash is a helper for when you have a []byte hash
func (w *Wallet) GetKeyByHash(publicKeyHash []byte) *Key {
	w.mu.RLock()
	defer w.mu.RUnlock()
	if len(publicKeyHash) == 0 {
		return nil
	}
	return w.keys[fmt.Sprintf("%x", publicKeyHash)]
}

// GetAllKeys returns a list of all keys in the wallet
func (w *Wallet) GetAllKeys() []*Key {
	w.mu.RLock()
	defer w.mu.RUnlock()
	
	keys := make([]*Key, 0, len(w.keys))
	for _, key := range w.keys {
		keys = append(keys, key)
	}
	return keys
}

// Get specific account types
func (w *Wallet) GetTokenAccount(u *url.URL) *TokenAccount {
	w.mu.RLock()
	defer w.mu.RUnlock()
	if u == nil {
		return nil
	}
	return w.tokenAccounts[*u]
}

func (w *Wallet) GetDataAccount(u *url.URL) *DataAccount {
	w.mu.RLock()
	defer w.mu.RUnlock()
	if u == nil {
		return nil
	}
	return w.dataAccounts[*u]
}

func (w *Wallet) GetTokenIssuer(u *url.URL) *TokenIssuer {
	w.mu.RLock()
	defer w.mu.RUnlock()
	if u == nil {
		return nil
	}
	return w.tokenIssuers[*u]
}

func (w *Wallet) GetLiteIdentity(u *url.URL) *LiteIdentity {
	w.mu.RLock()
	defer w.mu.RUnlock()
	if u == nil {
		return nil
	}
	return w.liteIdentities[*u]
}

func (w *Wallet) GetLiteTokenAccount(u *url.URL) *LiteTokenAccount {
	w.mu.RLock()
	defer w.mu.RUnlock()
	if u == nil {
		return nil
	}
	return w.liteTokenAccounts[*u]
}

func (w *Wallet) GetLiteDataAccount(u *url.URL) *LiteDataAccount {
	w.mu.RLock()
	defer w.mu.RUnlock()
	if u == nil {
		return nil
	}
	return w.liteDataAccounts[*u]
}

// Storage methods

func (w *Wallet) StoreAccount(account Account) {
	w.mu.Lock()
	defer w.mu.Unlock()
	
	if account == nil || account.GetURL() == nil {
		return
	}
	
	u := *account.GetURL()
	w.accounts[u] = account
	
	// Also store in type-specific map
	switch acc := account.(type) {
	case *ADI:
		w.adis[u] = acc
	case *TokenAccount:
		w.tokenAccounts[u] = acc
	case *DataAccount:
		w.dataAccounts[u] = acc
	case *TokenIssuer:
		w.tokenIssuers[u] = acc
	}
}

func (w *Wallet) StoreKeyBook(book *KeyBook) {
	w.mu.Lock()
	defer w.mu.Unlock()
	if book == nil || book.URL == nil {
		return
	}
	w.keyBooks[*book.URL] = book
}

func (w *Wallet) StoreKeyPage(page *KeyPage) {
	w.mu.Lock()
	defer w.mu.Unlock()
	if page == nil || page.URL == nil {
		return
	}
	w.keyPages[*page.URL] = page
}

func (w *Wallet) StoreKey(key *Key) {
	w.mu.Lock()
	defer w.mu.Unlock()
	if key == nil || len(key.PublicKeyHash) == 0 {
		return
	}
	hashHex := fmt.Sprintf("%x", key.PublicKeyHash)
	w.keys[hashHex] = key
}

func (w *Wallet) StoreLiteIdentity(identity *LiteIdentity) {
	w.mu.Lock()
	defer w.mu.Unlock()
	if identity == nil || identity.URL == nil {
		return
	}
	w.liteIdentities[*identity.URL] = identity
}

func (w *Wallet) StoreLiteTokenAccount(account *LiteTokenAccount) {
	w.mu.Lock()
	defer w.mu.Unlock()
	if account == nil || account.URL == nil {
		return
	}
	w.liteTokenAccounts[*account.URL] = account
}

func (w *Wallet) StoreLiteDataAccount(account *LiteDataAccount) {
	w.mu.Lock()
	defer w.mu.Unlock()
	if account == nil || account.URL == nil {
		return
	}
	w.liteDataAccounts[*account.URL] = account
}

// GetStats returns current wallet statistics
func (w *Wallet) GetStats() WalletStats {
	w.mu.RLock()
	defer w.mu.RUnlock()
	
	return WalletStats{
		TotalAccounts:      len(w.accounts),
		TotalADIs:          len(w.adis),
		TotalTokenAccounts: len(w.tokenAccounts),
		TotalDataAccounts:  len(w.dataAccounts),
		TotalTokenIssuers:  len(w.tokenIssuers),
		TotalLiteAccounts:  len(w.liteIdentities) + len(w.liteTokenAccounts) + len(w.liteDataAccounts),
		TotalKeyBooks:      len(w.keyBooks),
		TotalKeyPages:      len(w.keyPages),
		TotalKeys:          len(w.keys),
	}
}

// SetFundingAccount sets the primary funding account
func (w *Wallet) SetFundingAccount(identity *LiteIdentity) {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.fundingAccount = identity
}

// GetFundingAccount returns the primary funding account
func (w *Wallet) GetFundingAccount() *LiteIdentity {
	w.mu.RLock()
	defer w.mu.RUnlock()
	return w.fundingAccount
}

// StartFunding starts the funding manager with the given configuration
func (w *Wallet) StartFunding(config *FundingConfig) {
	w.mu.Lock()
	defer w.mu.Unlock()
	
	if w.fundingManager != nil {
		w.fundingManager.Stop()
	}
	
	w.fundingManager = NewFundingManager(w, config)
	w.fundingManager.Start()
}

// StopFunding stops the funding manager
func (w *Wallet) StopFunding() {
	w.mu.Lock()
	defer w.mu.Unlock()
	
	if w.fundingManager != nil {
		w.fundingManager.Stop()
		w.fundingManager = nil
	}
}

// GetFundingMetrics returns funding manager metrics
func (w *Wallet) GetFundingMetrics() *FundingMetrics {
	w.mu.RLock()
	defer w.mu.RUnlock()
	
	if w.fundingManager == nil {
		return nil
	}
	
	metrics := w.fundingManager.GetMetrics()
	return &metrics
}

// GetAllLiteIdentities returns all lite identities in the wallet
func (w *Wallet) GetAllLiteIdentities() []*LiteIdentity {
	w.mu.RLock()
	defer w.mu.RUnlock()
	
	identities := make([]*LiteIdentity, 0, len(w.liteIdentities))
	for _, identity := range w.liteIdentities {
		identities = append(identities, identity)
	}
	return identities
}

// GetAllKeyPages returns all key pages in the wallet
func (w *Wallet) GetAllKeyPages() []*KeyPage {
	w.mu.RLock()
	defer w.mu.RUnlock()
	
	pages := make([]*KeyPage, 0, len(w.keyPages))
	for _, page := range w.keyPages {
		pages = append(pages, page)
	}
	return pages
}

// CreateLiteAccount creates a new lite account with deterministic key generation
func (w *Wallet) CreateLiteAccount() (*LiteIdentity, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	
	// Generate deterministic seed for this account
	// Combine wallet seed with account counter
	accountSeed := sha256.Sum256(append(w.seed, byte(w.liteAccountCounter)))
	w.liteAccountCounter++
	
	// Use the seed to generate deterministic ed25519 keys
	// ED25519 requires exactly 32 bytes of seed
	privKey := ed25519.NewKeyFromSeed(accountSeed[:])
	pubKey := privKey.Public().(ed25519.PublicKey)
	
	// Create key hash
	keyHash := sha256.Sum256(pubKey)
	
	// Create lite URL
	liteURL := protocol.LiteAuthorityForKey(pubKey, protocol.SignatureTypeED25519)
	
	// Create Key structure
	key := &Key{
		Type:          protocol.SignatureTypeED25519,
		PublicKey:     pubKey,
		PrivateKey:    privKey,
		PublicKeyHash: keyHash[:],
	}
	
	// Store the key
	hashHex := fmt.Sprintf("%x", keyHash[:])
	w.keys[hashHex] = key
	
	// Create LiteIdentity
	identity := &LiteIdentity{
		URL:           liteURL,
		Key:           key,
		PublicKeyHash: keyHash[:20], // First 20 bytes for the hash
		Created:       true,
		LastUpdated:   time.Now(),
	}
	
	// Store the identity
	w.liteIdentities[*liteURL] = identity
	
	return identity, nil
}