package types

import (
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

// TransactionType is the type of a transaction.
// Deprecated: use protocol.TransactionType
type TransactionType = protocol.TransactionType

// TxType is an alias for TransactionType
// Deprecated: use protocol.TransactionType
type TxType = TransactionType

const (
	// TxTypeUnknown represents an unknown transaction type.
	// Deprecated: use protocol.TransactionTypeUnknown
	TxTypeUnknown = protocol.TransactionTypeUnknown
)

// User transactions
const (
	// TxTypeCreateIdentity creates an ADI, which produces a synthetic chain
	// create transaction.
	// Deprecated: use protocol.TransactionTypeCreateIdentity
	TxTypeCreateIdentity = protocol.TransactionTypeCreateIdentity

	// TxTypeCreateTokenAccount creates an ADI token account, which produces a
	// synthetic chain create transaction.
	// Deprecated: use protocol.TransactionTypeCreateTokenAccount
	TxTypeCreateTokenAccount = protocol.TransactionTypeCreateTokenAccount

	// TxTypeSendTokens transfers tokens between token accounts, which produces
	// a synthetic deposit tokens transaction.
	// Deprecated: use protocol.TransactionTypeSendTokens
	TxTypeSendTokens = protocol.TransactionTypeSendTokens

	// TxTypeCreateDataAccount creates an ADI Data Account, which produces a
	// synthetic chain create transaction.
	// Deprecated: use protocol.TransactionTypeCreateDataAccount
	TxTypeCreateDataAccount = protocol.TransactionTypeCreateDataAccount

	// TxTypeWriteData writes data to an ADI Data Account, which *does not*
	// produce a synthetic transaction.
	// Deprecated: use protocol.TransactionTypeWriteData
	TxTypeWriteData = protocol.TransactionTypeWriteData

	// TxTypeWriteDataTo writes data to a Lite Data Account, which produces a
	// synthetic write data transaction.
	// Deprecated: use protocol.TransactionTypeWriteDataTo
	TxTypeWriteDataTo = protocol.TransactionTypeWriteDataTo

	// TxTypeAcmeFaucet produces a synthetic deposit tokens transaction that
	// deposits ACME tokens into a lite token account.
	// Deprecated: use protocol.TransactionTypeAcmeFaucet
	TxTypeAcmeFaucet = protocol.TransactionTypeAcmeFaucet

	// TxTypeCreateToken creates a token issuer, which produces a synthetic
	// chain create transaction.
	// Deprecated: use protocol.TransactionTypeCreateToken
	TxTypeCreateToken = protocol.TransactionTypeCreateToken

	// TxTypeIssueTokens issues tokens to a token account, which produces a
	// synthetic token deposit transaction.
	// Deprecated: use protocol.TransactionTypeIssueTokens
	TxTypeIssueTokens = protocol.TransactionTypeIssueTokens

	// TxTypeBurnTokens burns tokens from a token account, which produces a
	// synthetic burn tokens transaction.
	// Deprecated: use protocol.TransactionTypeBurnTokens
	TxTypeBurnTokens = protocol.TransactionTypeBurnTokens

	// TxTypeCreateKeyPage creates a key page, which produces a synthetic chain
	// create transaction.
	// Deprecated: use protocol.TransactionTypeCreateKeyPage
	TxTypeCreateKeyPage = protocol.TransactionTypeCreateKeyPage

	// TxTypeCreateKeyBook creates a key book, which produces a synthetic chain
	// create transaction.
	// Deprecated: use protocol.TransactionTypeCreateKeyBook
	TxTypeCreateKeyBook = protocol.TransactionTypeCreateKeyBook

	// TxTypeAddCredits converts ACME tokens to credits, which produces a
	// synthetic deposit credits transaction.
	// Deprecated: use protocol.TransactionTypeAddCredits
	TxTypeAddCredits = protocol.TransactionTypeAddCredits

	// TxTypeUpdateKeyPage adds, removes, or updates keys in a key page, which
	// *does not* produce a synthetic transaction.
	// Deprecated: use protocol.TransactionTypeUpdateKeyPage
	TxTypeUpdateKeyPage = protocol.TransactionTypeUpdateKeyPage

	// TxTypeSignPending is used to sign a pending transaction.
	// Deprecated: use protocol.TransactionTypeSignPending
	TxTypeSignPending = protocol.TransactionTypeSignPending

	// TxTypeSignPending is used to sign a pending transaction.
	// Deprecated: use protocol.TransactionTypeSignPending
	TxTypeUpdateManager = protocol.TransactionTypeUpdateManager
)

// Synthetic transactions
const (
	// TxTypeSyntheticCreateChain creates or updates chains.
	// Deprecated: use protocol.TransactionTypeSyntheticCreateChain
	TxTypeSyntheticCreateChain = protocol.TransactionTypeSyntheticCreateChain

	// TxTypeSyntheticWriteData writes data to a data account.
	// Deprecated: use protocol.TransactionTypeSyntheticWriteData
	TxTypeSyntheticWriteData = protocol.TransactionTypeSyntheticWriteData

	// TxTypeSyntheticDepositTokens deposits tokens into token accounts.
	// Deprecated: use protocol.TransactionTypeSyntheticDepositTokens
	TxTypeSyntheticDepositTokens = protocol.TransactionTypeSyntheticDepositTokens

	// TxTypeSyntheticAnchor anchors one network to another.
	// Deprecated: use protocol.TransactionTypeSyntheticAnchor
	TxTypeSyntheticAnchor = protocol.TransactionTypeSyntheticAnchor

	// TxTypeSyntheticDepositCredits deposits credits into a credit holder.
	// Deprecated: use protocol.TransactionTypeSyntheticDepositCredits
	TxTypeSyntheticDepositCredits = protocol.TransactionTypeSyntheticDepositCredits

	// TxTypeSyntheticBurnTokens returns tokens to a token issuer's pool of
	// issuable tokens.
	// Deprecated: use protocol.TransactionTypeSyntheticBurnTokens
	TxTypeSyntheticBurnTokens = protocol.TransactionTypeSyntheticBurnTokens

	// TxTypeSyntheticMirror mirrors records from one network to another.
	// Deprecated: use protocol.TransactionTypeSyntheticMirror
	TxTypeSyntheticMirror = protocol.TransactionTypeSyntheticMirror

	// TxTypeSegWitDataEntry is a surrogate transaction segregated witness for
	// a WriteData transaction
	// Deprecated: use protocol.TransactionTypeSegWitDataEntry
	TxTypeSegWitDataEntry = protocol.TransactionTypeSegWitDataEntry
)

const (
	// TxTypeInternalGenesis initializes system chains.
	// Deprecated: use protocol.TransactionTypeInternalGenesis
	TxTypeInternalGenesis = protocol.TransactionTypeInternalGenesis

	// Deprecated: use protocol.TransactionTypeInternalSendTransactions
	TxTypeInternalSendTransactions = protocol.TransactionTypeInternalSendTransactions

	// TxTypeInternalTransactionsSigned notifies the executor of synthetic
	// transactions that have been signed.
	// Deprecated: use protocol.TransactionTypeInternalTransactionsSigned
	TxTypeInternalTransactionsSigned = protocol.TransactionTypeInternalTransactionsSigned

	// TxTypeInternalTransactionsSent notifies the executor of synthetic
	// transactions that have been sent.
	// Deprecated: use protocol.TransactionTypeInternalTransactionsSent
	TxTypeInternalTransactionsSent = protocol.TransactionTypeInternalTransactionsSent
)
