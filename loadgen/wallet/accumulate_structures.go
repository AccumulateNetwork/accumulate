package wallet

import (
	"time"

	"gitlab.com/accumulatenetwork/accumulate/pkg/url"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

// Protocol-defined structures that model Accumulate entities

// Account is the interface that all account types must implement
type Account interface {
	GetURL() *url.URL
	GetType() protocol.AccountType
	GetAuthorities() []*Authority
	IsCreated() bool
	GetLastUpdated() time.Time
	SetCreated(bool)
	SetLastUpdated(time.Time)
}

// BaseAccount contains common fields for all account types
type BaseAccount struct {
	URL         *url.URL
	Type        protocol.AccountType
	Authorities []*Authority
	Created     bool
	LastUpdated time.Time
}

func (a *BaseAccount) GetURL() *url.URL                { return a.URL }
func (a *BaseAccount) GetType() protocol.AccountType   { return a.Type }
func (a *BaseAccount) GetAuthorities() []*Authority    { return a.Authorities }
func (a *BaseAccount) IsCreated() bool                 { return a.Created }
func (a *BaseAccount) GetLastUpdated() time.Time       { return a.LastUpdated }
func (a *BaseAccount) SetCreated(created bool)         { a.Created = created }
func (a *BaseAccount) SetLastUpdated(t time.Time)      { a.LastUpdated = t }

// ADI represents an Accumulate Digital Identifier
type ADI struct {
	BaseAccount
	Name        string
	SubAccounts map[string]Account // Sub-account URL -> Account interface
}

// TokenAccount represents a token account
type TokenAccount struct {
	BaseAccount
	TokenURL *url.URL
	Balance  uint64 // Note: protocol uses big.Int, but uint64 is sufficient for load testing
}

// DataAccount represents a data account
type DataAccount struct {
	BaseAccount
}

// TokenIssuer represents a token issuer account
type TokenIssuer struct {
	BaseAccount
	Symbol      string
	Precision   uint64
	SupplyLimit *uint64  // nil for unlimited, protocol uses *big.Int
	Issued      uint64   // Protocol uses big.Int, but uint64 is sufficient for load testing
	Properties  *url.URL // URL to properties document (optional)
}

// LiteIdentity represents a lite identity (the root account for lite accounts)
type LiteIdentity struct {
	URL           *url.URL
	Key           *Key   // The cryptographic key for this identity
	PublicKeyHash []byte // First 20 bytes of key hash (used in URL)
	CreditBalance uint64
	LastUsedOn    uint64                       // Nonce for replay protection
	TokenAccounts map[string]*LiteTokenAccount // Token URL -> Account
	DataAccounts  map[string]*LiteDataAccount  // Chain ID -> Account
	Created       bool
	LastUpdated   time.Time
}

// LiteTokenAccount represents a lite token account
type LiteTokenAccount struct {
	URL            *url.URL
	IdentityURL    *url.URL // The parent lite identity
	TokenIssuerURL *url.URL // URL of the token issuer (e.g., acc://ACME or acc://alice.acme/my-token)
	Balance        uint64   // Protocol uses big.Int, but uint64 is sufficient for load testing
	LockHeight     uint64   // Major block height after which balance can be transferred
	Created        bool
	LastUpdated    time.Time
}

// LiteDataAccount represents a lite data account
type LiteDataAccount struct {
	URL         *url.URL
	ChainID     []byte // 32-byte chain ID computed from first entry
	Created     bool
	LastUpdated time.Time
}

// Authority represents an authority entry in AccountAuth
type Authority struct {
	URL      *url.URL
	KeyBook  *KeyBook
	Priority int  // Index in authorities array
	Disabled bool
}

// KeyBook represents a key book (authority container)
type KeyBook struct {
	URL         *url.URL
	ADI         string // Parent ADI name
	Name        string // Book name within ADI
	BookType    protocol.BookType
	PageCount   uint64
	Pages       []*KeyPage // Indexed array of pages
	Authorities []*Authority
	Created     bool
	LastUpdated time.Time
}

// KeyPage represents a key page (signature collection)
type KeyPage struct {
	URL                  *url.URL
	BookURL              *url.URL
	Index                uint64
	Keys                 []*KeySpec                    // Can be key hashes or delegates to other signers
	AcceptThreshold      uint64
	RejectThreshold      uint64
	ResponseThreshold    uint64
	BlockThreshold       uint64
	CreditBalance        uint64
	Version              uint64                        // For tracking updates
	TransactionBlacklist *protocol.AllowedTransactions // Optional transaction restrictions
	Created              bool
	LastUpdated          time.Time
}

// KeySpec represents a signer entry in a key page
type KeySpec struct {
	PublicKeyHash []byte   // Hash of the public key (if direct key)
	LastUsedOn    uint64   // Nonce for replay protection
	Delegate      *url.URL // URL of delegated signer (e.g., another KeyBook)
}

// Key represents an actual cryptographic key
type Key struct {
	Type          protocol.SignatureType // ED25519, BTC, ETH, RSA, ECDSA, etc.
	PublicKey     []byte                 // Raw public key bytes
	PrivateKey    []byte                 // Raw private key bytes (optional, for wallet-managed keys)
	PublicKeyHash []byte                 // Hash of public key (variable length, depends on Type)
	Created       time.Time
}