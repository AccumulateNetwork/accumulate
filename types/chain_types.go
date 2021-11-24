package types

import (
	"encoding/json"
	"fmt"
	"strings"
)

// ChainType is the type of a chain.
type ChainType uint64

const (
	// ChainTypeUnknown represents an unknown chain type.
	ChainTypeUnknown = ChainType(iota)
)

const (
	// ChainTypeAnchor is one or more Merkle DAG anchors.
	ChainTypeAnchor ChainType = 1

	// ChainTypeIdentity is an Identity chain, aka an ADI.
	ChainTypeIdentity ChainType = 2

	// ChainTypeTokenIssuer is a Token Issuer chain.
	ChainTypeTokenIssuer ChainType = 3

	// ChainTypeTokenAccount is an ADI Token Account chain.
	ChainTypeTokenAccount ChainType = 4

	// ChainTypeLiteTokenAccount is a Lite Token Account chain.
	ChainTypeLiteTokenAccount ChainType = 5

	// ChainTypeTransactionReference is a reference to a transaction.
	ChainTypeTransactionReference ChainType = 6

	// ChainTypeTransaction is a completed transaction.
	ChainTypeTransaction ChainType = 7

	// ChainTypeTransaction is a pending transaction.
	ChainTypePendingTransaction ChainType = 8

	// ChainTypeKeyPage is a key page chain.
	ChainTypeKeyPage ChainType = 9

	// ChainTypeKeyBook is a key book chain.
	ChainTypeKeyBook ChainType = 10

	// ChainTypeDataAccount is an ADI Data Account chain.
	ChainTypeDataAccount ChainType = 11

	// ChainTypeLiteDataAccount is a Lite Data Account chain.
	ChainTypeLiteDataAccount ChainType = 12

	// chainMax needs to be set to the last type in the list above
	chainMax = ChainTypeLiteDataAccount
)

// ID returns the chain type ID
func (t ChainType) ID() uint64 { return uint64(t) }

// IsTransaction returns true if the chain type is a transaction.
func (t ChainType) IsTransaction() bool {
	switch t {
	case ChainTypeTransaction, ChainTypeTransactionReference, ChainTypePendingTransaction:
		return true
	default:
		return false
	}
}

// String returns the name of the chain type
func (t ChainType) String() string {
	switch t {
	case ChainTypeUnknown:
		return "unknown"
	case ChainTypeIdentity:
		return "identity"
	case ChainTypeTokenIssuer:
		return "token"
	case ChainTypeTokenAccount:
		return "tokenAccount"
	case ChainTypeLiteTokenAccount:
		return "liteTokenAccount"
	case ChainTypeTransactionReference:
		return "transactionReference"
	case ChainTypeTransaction:
		return "transaction"
	case ChainTypePendingTransaction:
		return "pendingTransaction"
	case ChainTypeKeyPage:
		return "keyPage"
	case ChainTypeKeyBook:
		return "keyBook"
	case ChainTypeDataAccount:
		return "dataAccount"
	case ChainTypeLiteDataAccount:
		return "liteDataAccount"
	default:
		return fmt.Sprintf("ChainType:%d", t)
	}
}

// Name is an alias for String
// Deprecated: use String
func (t ChainType) Name() string { return t.String() }

var chainByName = map[string]ChainType{}

func init() {
	for t := ChainTypeUnknown; t <= chainMax; t++ {
		chainByName[t.String()] = t
	}
}

func (t *ChainType) UnmarshalJSON(data []byte) error {
	var s string
	err := json.Unmarshal(data, &s)
	if err != nil {
		return err
	}

	var ok bool
	*t, ok = chainByName[s]
	if !ok || strings.ContainsRune(t.String(), ':') {
		return fmt.Errorf("invalid transaction type %q", s)
	}
	return nil
}

func (t ChainType) MarshalJSON() ([]byte, error) {
	return json.Marshal(t.String())
}
