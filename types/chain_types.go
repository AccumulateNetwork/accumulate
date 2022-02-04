package types

import (
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

// AccountType is the type of an account.
// Deprecated: use protocol.AccountType
type AccountType = protocol.AccountType

const (
	// AccountTypeUnknown represents an unknown account type.
	// Deprecated: use protocol.AccountTypeUnknown
	AccountTypeUnknown = protocol.AccountTypeUnknown

	// AccountTypeAnchor is one or more Merkle DAG anchors.
	// Deprecated: use protocol.AccountTypeAnchor
	AccountTypeAnchor = protocol.AccountTypeAnchor

	// AccountTypeIdentity is an Identity account, aka an ADI.
	// Deprecated: use protocol.AccountTypeIdentity
	AccountTypeIdentity = protocol.AccountTypeIdentity

	// AccountTypeTokenIssuer is a Token Issuer account.
	// Deprecated: use protocol.AccountTypeTokenIssuer
	AccountTypeTokenIssuer = protocol.AccountTypeTokenIssuer

	// AccountTypeTokenAccount is an ADI Token Account.
	// Deprecated: use protocol.AccountTypeTokenAccount
	AccountTypeTokenAccount = protocol.AccountTypeTokenAccount

	// AccountTypeLiteTokenAccount is a Lite Token Account.
	// Deprecated: use protocol.AccountTypeLiteTokenAccount
	AccountTypeLiteTokenAccount = protocol.AccountTypeLiteTokenAccount

	// AccountTypeTransaction is a completed transaction.
	// Deprecated: use protocol.AccountTypeTransaction
	AccountTypeTransaction = protocol.AccountTypeTransaction

	// AccountTypePendingTransaction is a pending transaction.
	// Deprecated: use protocol.AccountTypePendingTransaction
	AccountTypePendingTransaction = protocol.AccountTypePendingTransaction

	// AccountTypeKeyPage is a Key Page account.
	// Deprecated: use protocol.AccountTypeKeyPage
	AccountTypeKeyPage = protocol.AccountTypeKeyPage

	// AccountTypeKeyBook is a Key Book account.
	// Deprecated: use protocol.AccountTypeKeyBook
	AccountTypeKeyBook = protocol.AccountTypeKeyBook

	// AccountTypeDataAccount is an ADI Data Account.
	// Deprecated: use protocol.AccountTypeDataAccount
	AccountTypeDataAccount = protocol.AccountTypeDataAccount

	// AccountTypeLiteDataAccount is a Lite Data Account.
	// Deprecated: use protocol.AccountTypeLiteDataAccount
	AccountTypeLiteDataAccount = protocol.AccountTypeLiteDataAccount

	// AccountTypeInternalLedger is a ledger that tracks the state of internal
	// operations.
	// Deprecated: use protocol.AccountTypeInternalLedger
	AccountTypeInternalLedger = protocol.AccountTypeInternalLedger
)
