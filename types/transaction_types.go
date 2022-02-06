package types

import (
	"encoding/json"
	"fmt"
	"strings"

	"gitlab.com/accumulatenetwork/accumulate/internal/encoding"
)

// TransactionType is the type of a transaction.
// Deprecated: use protocol.TransactionType
type TransactionType uint64

// TxType is an alias for TransactionType
// Deprecated: use protocol.TransactionType
type TxType = TransactionType

const (
	// TxTypeUnknown represents an unknown transaction type.
	// Deprecated: use protocol.TransactionTypeUnknown
	TxTypeUnknown TransactionType = 0x00

	// txMaxUser is the highest number reserved for user transactions.
	// Deprecated: use protocol.TransactionMaxUser
	txMaxUser TransactionType = 0x2F

	// txMaxSynthetic is the highest number reserved for synthetic transactions.
	// Deprecated: use protocol.TransactionaxSynthetic
	txMaxSynthetic TransactionType = 0x5F

	// txMaxInternal is the highest number reserved for internal transactions.
	// Deprecated: use protocol.TransactionMaxInternal
	txMaxInternal TransactionType = 0xFF
)

// User transactions
const (
	// TxTypeCreateIdentity creates an ADI, which produces a synthetic chain
	// create transaction.
	// Deprecated: use protocol.TransactionTypeCreateIdentity
	TxTypeCreateIdentity TransactionType = 0x01

	// TxTypeCreateTokenAccount creates an ADI token account, which produces a
	// synthetic chain create transaction.
	// Deprecated: use protocol.TransactionTypeCreateTokenAccount
	TxTypeCreateTokenAccount TransactionType = 0x02

	// TxTypeSendTokens transfers tokens between token accounts, which produces
	// a synthetic deposit tokens transaction.
	// Deprecated: use protocol.TransactionTypeSendTokens
	TxTypeSendTokens TransactionType = 0x03

	// TxTypeCreateDataAccount creates an ADI Data Account, which produces a
	// synthetic chain create transaction.
	// Deprecated: use protocol.TransactionTypeCreateDataAccount
	TxTypeCreateDataAccount TransactionType = 0x04

	// TxTypeWriteData writes data to an ADI Data Account, which *does not*
	// produce a synthetic transaction.
	// Deprecated: use protocol.TransactionTypeWriteData
	TxTypeWriteData TransactionType = 0x05

	// TxTypeWriteDataTo writes data to a Lite Data Account, which produces a
	// synthetic write data transaction.
	// Deprecated: use protocol.TransactionTypeWriteDataTo
	TxTypeWriteDataTo TransactionType = 0x06

	// TxTypeAcmeFaucet produces a synthetic deposit tokens transaction that
	// deposits ACME tokens into a lite token account.
	// Deprecated: use protocol.TransactionTypeAcmeFaucet
	TxTypeAcmeFaucet TransactionType = 0x07

	// TxTypeCreateToken creates a token issuer, which produces a synthetic
	// chain create transaction.
	// Deprecated: use protocol.TransactionTypeCreateToken
	TxTypeCreateToken TransactionType = 0x08

	// TxTypeIssueTokens issues tokens to a token account, which produces a
	// synthetic token deposit transaction.
	// Deprecated: use protocol.TransactionTypeIssueTokens
	TxTypeIssueTokens TransactionType = 0x09

	// TxTypeBurnTokens burns tokens from a token account, which produces a
	// synthetic burn tokens transaction.
	// Deprecated: use protocol.TransactionTypeBurnTokens
	TxTypeBurnTokens TransactionType = 0x0A

	// TxTypeCreateKeyPage creates a key page, which produces a synthetic chain
	// create transaction.
	// Deprecated: use protocol.TransactionTypeCreateKeyPage
	TxTypeCreateKeyPage TransactionType = 0x0C

	// TxTypeCreateKeyBook creates a key book, which produces a synthetic chain
	// create transaction.
	// Deprecated: use protocol.TransactionTypeCreateKeyBook
	TxTypeCreateKeyBook TransactionType = 0x0D

	// TxTypeAddCredits converts ACME tokens to credits, which produces a
	// synthetic deposit credits transaction.
	// Deprecated: use protocol.TransactionTypeAddCredits
	TxTypeAddCredits TransactionType = 0x0E

	// TxTypeUpdateKeyPage adds, removes, or updates keys in a key page, which
	// *does not* produce a synthetic transaction.
	// Deprecated: use protocol.TransactionTypeUpdateKeyPage
	TxTypeUpdateKeyPage TransactionType = 0x0F

	// TxTypeSignPending is used to sign a pending transaction.
	// Deprecated: use protocol.TransactionTypeSignPending
	TxTypeSignPending TransactionType = 0x30

	// TxTypeSignPending is used to sign a pending transaction.
	// Deprecated: use protocol.TransactionTypeSignPending
	TxTypeUpdateManager TransactionType = 0x40
)

// Synthetic transactions
const (
	// TxTypeSyntheticCreateChain creates or updates chains.
	// Deprecated: use protocol.TransactionTypeSyntheticCreateChain
	TxTypeSyntheticCreateChain TransactionType = 0x31

	// TxTypeSyntheticWriteData writes data to a data account.
	// Deprecated: use protocol.TransactionTypeSyntheticWriteData
	TxTypeSyntheticWriteData TransactionType = 0x32

	// TxTypeSyntheticDepositTokens deposits tokens into token accounts.
	// Deprecated: use protocol.TransactionTypeSyntheticDepositTokens
	TxTypeSyntheticDepositTokens TransactionType = 0x33

	// TxTypeSyntheticAnchor anchors one network to another.
	// Deprecated: use protocol.TransactionTypeSyntheticAnchor
	TxTypeSyntheticAnchor TransactionType = 0x34

	// TxTypeSyntheticDepositCredits deposits credits into a credit holder.
	// Deprecated: use protocol.TransactionTypeSyntheticDepositCredits
	TxTypeSyntheticDepositCredits TransactionType = 0x35

	// TxTypeSyntheticBurnTokens returns tokens to a token issuer's pool of
	// issuable tokens.
	// Deprecated: use protocol.TransactionTypeSyntheticBurnTokens
	TxTypeSyntheticBurnTokens TransactionType = 0x36

	// TxTypeSyntheticMirror mirrors records from one network to another.
	// Deprecated: use protocol.TransactionTypeSyntheticMirror
	TxTypeSyntheticMirror TransactionType = 0x38

	// TxTypeSegWitDataEntry is a surrogate transaction segregated witness for
	// a WriteData transaction
	// Deprecated: use protocol.TransactionTypeSegWitDataEntry
	TxTypeSegWitDataEntry TransactionType = 0x39
)

const (
	// TxTypeInternalGenesis initializes system chains.
	// Deprecated: use protocol.TransactionTypeInternalGenesis
	TxTypeInternalGenesis TransactionType = 0x60

	// Deprecated: use protocol.TransactionTypeInternalSendTransactions
	TxTypeInternalSendTransactions TransactionType = 0x61

	// TxTypeInternalTransactionsSigned notifies the executor of synthetic
	// transactions that have been signed.
	// Deprecated: use protocol.TransactionTypeInternalTransactionsSigned
	TxTypeInternalTransactionsSigned TransactionType = 0x62

	// TxTypeInternalTransactionsSent notifies the executor of synthetic
	// transactions that have been sent.
	// Deprecated: use protocol.TransactionTypeInternalTransactionsSent
	TxTypeInternalTransactionsSent TransactionType = 0x63
)

// IsUser returns true if the transaction type is user.
func (t TransactionType) IsUser() bool { return TxTypeUnknown < t && t <= txMaxUser }

// IsSynthetic returns true if the transaction type is synthetic.
func (t TransactionType) IsSynthetic() bool { return txMaxUser < t && t <= txMaxSynthetic }

// IsInternal returns true if the transaction type is internal.
func (t TransactionType) IsInternal() bool { return txMaxSynthetic < t && t <= txMaxInternal }

// ID returns the transaction type ID
func (t TransactionType) ID() uint64 { return uint64(t) }

// String returns the name of the transaction type
func (t TransactionType) String() string {
	switch t {
	case TxTypeUnknown:
		return "unknown"
	case TxTypeCreateIdentity:
		return "createIdentity"
	case TxTypeCreateTokenAccount:
		return "createTokenAccount"
	case TxTypeSendTokens:
		return "sendTokens"
	case TxTypeCreateDataAccount:
		return "createDataAccount"
	case TxTypeWriteData:
		return "writeData"
	case TxTypeWriteDataTo:
		return "writeDataTo"
	case TxTypeAcmeFaucet:
		return "acmeFaucet"
	case TxTypeCreateToken:
		return "createToken"
	case TxTypeIssueTokens:
		return "issueTokens"
	case TxTypeBurnTokens:
		return "burnTokens"
	case TxTypeCreateKeyPage:
		return "createKeyPage"
	case TxTypeCreateKeyBook:
		return "createKeyBook"
	case TxTypeAddCredits:
		return "addCredits"
	case TxTypeUpdateKeyPage:
		return "updateKeyPage"
	case TxTypeSignPending:
		return "signPending"
	case TxTypeUpdateManager:
		return "updateManager"

	case TxTypeSyntheticCreateChain:
		return "syntheticCreateChain"
	case TxTypeSyntheticWriteData:
		return "syntheticWriteData"
	case TxTypeSyntheticDepositTokens:
		return "syntheticDepositTokens"
	case TxTypeSyntheticAnchor:
		return "syntheticAnchor"
	case TxTypeSyntheticDepositCredits:
		return "syntheticDepositCredits"
	case TxTypeSyntheticBurnTokens:
		return "syntheticBurnTokens"
	case TxTypeSyntheticMirror:
		return "syntheticMirror"
	case TxTypeSegWitDataEntry:
		return "segWitDataEntry"

	case TxTypeInternalGenesis:
		return "genesis"
	case TxTypeInternalSendTransactions:
		return "sendTransactions"
	case TxTypeInternalTransactionsSigned:
		return "transactionsSigned"
	case TxTypeInternalTransactionsSent:
		return "transactionsSent"

	default:
		return fmt.Sprintf("TransactionType:%d", t)
	}
}

// Name is an alias for String
// Deprecated: use String
func (t TransactionType) Name() string { return t.String() }

var txByName = map[string]TransactionType{}

func init() {
	for t := TxTypeUnknown; t < txMaxInternal; t++ {
		txByName[t.String()] = t
	}
}

func (t *TransactionType) UnmarshalJSON(data []byte) error {
	var s string
	err := json.Unmarshal(data, &s)
	if err != nil {
		return err
	}

	var ok bool
	*t, ok = txByName[s]
	if !ok || strings.ContainsRune(t.String(), ':') {
		return fmt.Errorf("invalid transaction type %q", s)
	}
	return nil
}

func (t TransactionType) MarshalJSON() ([]byte, error) {
	return json.Marshal(t.String())
}

func (t TransactionType) BinarySize() int {
	return encoding.UvarintBinarySize(t.ID())
}

func (t TransactionType) MarshalBinary() ([]byte, error) {
	return encoding.UvarintMarshalBinary(t.ID()), nil
}

func (t *TransactionType) UnmarshalBinary(data []byte) error {
	v, err := encoding.UvarintUnmarshalBinary(data)
	if err != nil {
		return err
	}

	*t = TransactionType(v)
	return nil
}
