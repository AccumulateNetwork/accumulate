package types

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/AccumulateNetwork/accumulate/internal/encoding"
)

// TransactionType is the type of a transaction.
type TransactionType uint64

// TxType is an alias for TransactionType
// Deprecated: use TransactionType
type TxType = TransactionType

const (
	// TxTypeUnknown represents an unknown transaction type.
	TxTypeUnknown TransactionType = 0x00

	// txMaxUser is the highest number reserved for user transactions.
	txMaxUser TransactionType = 0x2F

	// txMaxSynthetic is the highest number reserved for synthetic transactions.
	txMaxSynthetic TransactionType = 0x5F

	// txMaxInternal is the highest number reserved for internal transactions.
	txMaxInternal TransactionType = 0xFF
)

// User transactions
const (
	// TxTypeCreateIdentity creates an ADI, which produces a synthetic chain
	// create transaction.
	TxTypeCreateIdentity TransactionType = 0x01

	// TxTypeCreateTokenAccount creates an ADI token account, which produces a
	// synthetic chain create transaction.
	TxTypeCreateTokenAccount TransactionType = 0x02

	// TxTypeSendTokens transfers tokens between token accounts, which produces
	// a synthetic deposit tokens transaction.
	TxTypeSendTokens TransactionType = 0x03

	// TxTypeCreateDataAccount creates an ADI Data Account, which produces a
	// synthetic chain create transaction.
	TxTypeCreateDataAccount TransactionType = 0x04

	// TxTypeWriteData writes data to an ADI Data Account, which *does not*
	// produce a synthetic transaction.
	TxTypeWriteData TransactionType = 0x05

	// TxTypeWriteDataTo writes data to a Lite Data Account, which produces a
	// synthetic write data transaction.
	TxTypeWriteDataTo TransactionType = 0x06

	// TxTypeAcmeFaucet produces a synthetic deposit tokens transaction that
	// deposits ACME tokens into a lite account.
	TxTypeAcmeFaucet TransactionType = 0x07

	// TxTypeCreateToken creates a token issuer, which produces a synthetic
	// chain create transaction.
	TxTypeCreateToken TransactionType = 0x08

	// TxTypeIssueTokens issues tokens to a token account, which produces a
	// synthetic token deposit transaction.
	TxTypeIssueTokens TransactionType = 0x09

	// TxTypeBurnTokens burns tokens from a token account, which produces a
	// synthetic burn tokens transaction.
	TxTypeBurnTokens TransactionType = 0x0A

	// TxTypeCreateKeyPage creates a key page, which produces a synthetic chain
	// create transaction.
	TxTypeCreateKeyPage TransactionType = 0x0C

	// TxTypeCreateKeyBook creates a key book, which produces a synthetic chain
	// create transaction.
	TxTypeCreateKeyBook TransactionType = 0x0D

	// TxTypeAddCredits converts ACME tokens to credits, which produces a
	// synthetic deposit credits transaction.
	TxTypeAddCredits TransactionType = 0x0E

	// TxTypeUpdateKeyPage adds, removes, or updates keys in a key page, which
	// *does not* produce a synthetic transaction.
	TxTypeUpdateKeyPage TransactionType = 0x0F

	// TxTypeSignPending is used to sign a pending transaction.
	TxTypeSignPending TransactionType = 0x30
)

// Synthetic transactions
const (
	// TxTypeSyntheticCreateChain creates or updates chains.
	TxTypeSyntheticCreateChain TransactionType = 0x31

	// TxTypeSyntheticWriteData writes data to a data account.
	TxTypeSyntheticWriteData TransactionType = 0x32

	// TxTypeSyntheticDepositTokens deposits tokens into token accounts.
	TxTypeSyntheticDepositTokens TransactionType = 0x33

	// TxTypeSyntheticAnchor anchors one network to another.
	TxTypeSyntheticAnchor TransactionType = 0x34

	// TxTypeSyntheticDepositCredits deposits credits into a credit holder.
	TxTypeSyntheticDepositCredits TransactionType = 0x35

	// TxTypeSyntheticBurnTokens returns tokens to a token issuer's pool of
	// issuable tokens.
	TxTypeSyntheticBurnTokens TransactionType = 0x36

	// TxTypeSyntheticMirror mirrors records from one network to another.
	TxTypeSyntheticMirror TransactionType = 0x38

	// TxTypeSegWitDataEntry is a surrogate transaction segregated witness for
	// a WriteData transaction
	TxTypeSegWitDataEntry TransactionType = 0x39
)

const (
	// TxTypeInternalGenesis initializes system chains.
	TxTypeInternalGenesis TransactionType = 0x60

	TxTypeInternalSendTransactions TransactionType = 0x61

	// TxTypeInternalTransactionsSigned notifies the executor of synthetic
	// transactions that have been signed.
	TxTypeInternalTransactionsSigned TransactionType = 0x62

	// TxTypeInternalTransactionsSent notifies the executor of synthetic
	// transactions that have been sent.
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
