package liteclient

import (
	"time"
	"gitlab.com/accumulatenetwork/accumulate/pkg/types/merkle"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
	v2api "gitlab.com/accumulatenetwork/accumulate/pkg/client/api/v2"
)

// VerifiedAccount contains an account URL and its cryptographic proof.
type VerifiedAccount struct {
	Url     string
	Receipt *merkle.Receipt
	Height  int64
}

// Transaction represents transaction data (unified struct)
type Transaction struct {
	TxID      string
	Type      string
	Status    string
	Timestamp int64
	Amount    string
	From      string
	To        string
	Account   string
	Height    int64
	Data      interface{} // Raw transaction data
}

// AccountData represents comprehensive account information
type AccountData struct {
	URL         string                    `json:"url"`
	Type        protocol.AccountType      `json:"type"`
	TypeName    string                    `json:"typeName"`
	Data        interface{}               `json:"data"`        // The actual account struct
	Balance     string                    `json:"balance"`     // For convenience
	TokenURL    string                    `json:"tokenUrl"`    // For convenience
	LastUpdated time.Time                 `json:"lastUpdated"`
	RawResponse *v2api.ChainQueryResponse `json:"rawResponse"` // Original API response
}

// CacheStats represents cache statistics for the public API
type CacheStats struct {
	AccountDataEntries int           `json:"accountDataEntries"`
	TransactionEntries int           `json:"transactionEntries"`
	BalanceEntries     int           `json:"balanceEntries"`
	ProofEntries       int           `json:"proofEntries"`
	TotalEntries       int           `json:"totalEntries"`
	HitRate            float64       `json:"hitRate"`
	AverageAge         time.Duration `json:"averageAge"`
}
