package types

import (
	"encoding/json"
	"fmt"
	"strings"
)

// AccountType is the type of an account.
type AccountType uint64

const (
	// AccountTypeUnknown represents an unknown account type.
	AccountTypeUnknown AccountType = 0

	// AccountTypeAnchor is one or more Merkle DAG anchors.
	AccountTypeAnchor AccountType = 1

	// AccountTypeIdentity is an Identity account, aka an ADI.
	AccountTypeIdentity AccountType = 2

	// AccountTypeTokenIssuer is a Token Issuer account.
	AccountTypeTokenIssuer AccountType = 3

	// AccountTypeTokenAccount is an ADI Token Account.
	AccountTypeTokenAccount AccountType = 4

	// AccountTypeLiteTokenAccount is a Lite Token Account.
	AccountTypeLiteTokenAccount AccountType = 5

	// AccountTypeTransaction is a completed transaction.
	AccountTypeTransaction AccountType = 7

	// AccountTypePendingTransaction is a pending transaction.
	AccountTypePendingTransaction AccountType = 8

	// AccountTypeKeyPage is a Key Page account.
	AccountTypeKeyPage AccountType = 9

	// AccountTypeKeyBook is a Key Book account.
	AccountTypeKeyBook AccountType = 10

	// AccountTypeDataAccount is an ADI Data Account.
	AccountTypeDataAccount AccountType = 11

	// AccountTypeLiteDataAccount is a Lite Data Account.
	AccountTypeLiteDataAccount AccountType = 12

	AccountTypeValidator AccountType = 13

	// AccountTypeInternalLedger is a ledger that tracks the state of internal
	// operations.
	AccountTypeInternalLedger AccountType = 14

	// accountMax needs to be set to the last type in the list above
	accountMax = AccountTypeInternalLedger
)

// ID returns the account type ID
func (t AccountType) ID() uint64 { return uint64(t) }

// IsTransaction returns true if the account type is a transaction.
func (t AccountType) IsTransaction() bool {
	switch t {
	case AccountTypeTransaction, AccountTypePendingTransaction:
		return true
	default:
		return false
	}
}

// String returns the name of the account type
func (t AccountType) String() string {
	switch t {
	case AccountTypeUnknown:
		return "unknown"
	case AccountTypeIdentity:
		return "identity"
	case AccountTypeTokenIssuer:
		return "token"
	case AccountTypeTokenAccount:
		return "tokenAccount"
	case AccountTypeLiteTokenAccount:
		return "liteTokenAccount"
	case AccountTypeTransaction:
		return "transaction"
	case AccountTypePendingTransaction:
		return "pendingTransaction"
	case AccountTypeKeyPage:
		return "keyPage"
	case AccountTypeKeyBook:
		return "keyBook"
	case AccountTypeDataAccount:
		return "dataAccount"
	case AccountTypeLiteDataAccount:
		return "liteDataAccount"
	case AccountTypeValidator:
		return "validator"
	default:
		return fmt.Sprintf("ChainType:%d", t)
	}
}

// Name is an alias for String
// Deprecated: use String
func (t AccountType) Name() string { return t.String() }

var chainByName = map[string]AccountType{}

func init() {
	for t := AccountTypeUnknown; t <= accountMax; t++ {
		chainByName[t.String()] = t
	}
}

func (t *AccountType) UnmarshalJSON(data []byte) error {
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

func (t AccountType) MarshalJSON() ([]byte, error) {
	return json.Marshal(t.String())
}
