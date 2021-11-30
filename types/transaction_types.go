package types

import (
	"encoding/json"
	"fmt"
	"strings"
)

// TransactionType is the type of a transaction.
type TransactionType uint64

// TxType is an alias for TransactionType
// Deprecated: use TransactionType
type TxType = TransactionType

const (
	// TxTypeUnknown represents an unknown transaction type.
	TxTypeUnknown TransactionType = 0x00

	// txSynthetic marks the boundary between user and system transactions.
	txSynthetic TransactionType = 0x30

	// txMax is the last defined transaction type.
	txMax = TxTypeSyntheticGenesis
)

// User transactions
const (
	// TxTypeCreateIdentity creates an ADI, which produces a synthetic chain
	// create transaction.
	TxTypeCreateIdentity TransactionType = 0x01

	// TxTypeCreateTokenAccount creates an ADI token account, which produces a
	// synthetic chain create transaction.
	TxTypeCreateTokenAccount TransactionType = 0x02

	// TxTypeWithdrawTokens transfers tokens between token accounts, which
	// produces a synthetic deposit tokens transaction.
	TxTypeWithdrawTokens TransactionType = 0x03

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
)

// System transactions
const (
	// TxTypeSyntheticCreateChain creates or updates chains.
	TxTypeSyntheticCreateChain TransactionType = 0x31

	// TxTypeSyntheticWriteData writes data to a data account.
	TxTypeSyntheticWriteData TransactionType = 0x32

	// TxTypeSyntheticDepositTokens deposits tokens into token accounts.
	TxTypeSyntheticDepositTokens TransactionType = 0x33

	// TxTypeSyntheticDepositCredits deposits credits into a credit holder.
	TxTypeSyntheticDepositCredits TransactionType = 0x35

	// TxTypeSyntheticBurnTokens returns tokens to a token issuer's pool of
	// issuable tokens.
	TxTypeSyntheticBurnTokens TransactionType = 0x36

	// TxTypeSyntheticGenesis initializes system chains.
	TxTypeSyntheticGenesis TransactionType = 0x37
)

// IsSynthetic returns true if the transaction type is synthetic.
func (t TransactionType) IsSynthetic() bool { return t >= txSynthetic }

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
	case TxTypeWithdrawTokens:
		return "withdrawTokens"
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
	case TxTypeSyntheticCreateChain:
		return "syntheticCreateChain"
	case TxTypeSyntheticWriteData:
		return "syntheticWriteData"
	case TxTypeSyntheticDepositTokens:
		return "syntheticDepositTokens"
	case TxTypeSyntheticDepositCredits:
		return "syntheticDepositCredits"
	case TxTypeSyntheticBurnTokens:
		return "syntheticBurnTokens"
	case TxTypeSyntheticGenesis:
		return "syntheticGenesis"
	default:
		return fmt.Sprintf("TransactionType:%d", t)
	}
}

// Name is an alias for String
// Deprecated: use String
func (t TransactionType) Name() string { return t.String() }

var txByName = map[string]TransactionType{}

func init() {
	for t := TxTypeUnknown; t < txMax; t++ {
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
