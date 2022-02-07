package protocol

import (
	"fmt"
)

// Fee is the unit cost of a transaction.
type Fee int

func (n Fee) AsInt() int {
	return int(n)
}

// Fee Schedule
const (
	// FeeFailedMaximum $0.01
	FeeFailedMaximum Fee = 100

	// FeeCreateIdentity $5.00 = 50000 credits @ 0.0001 / credit.
	FeeCreateIdentity Fee = 50000

	// FeeCreateTokenAccount $0.25
	FeeCreateTokenAccount Fee = 2500

	// FeeSendTokens $0.03
	FeeSendTokens Fee = 300

	// FeeCreateDataAccount $.25
	FeeCreateDataAccount Fee = 2500

	// FeeWriteData $0.001 / 256 bytes
	FeeWriteData Fee = 10

	// FeeWriteDataTo $0.001 / 256 bytes
	FeeWriteDataTo Fee = 10

	// FeeCreateToken $50.00
	FeeCreateToken Fee = 500000

	// FeeIssueTokens equiv. to token send @ $0.03
	FeeIssueTokens Fee = 300

	// FeeAcmeFaucet free
	FeeAcmeFaucet Fee = 0

	// FeeBurnTokens equiv. to token send
	FeeBurnTokens Fee = 300

	// FeeCreateKeyPage $1.00
	FeeCreateKeyPage Fee = 10000

	// FeeCreateKeyBook $1.00
	FeeCreateKeyBook Fee = 10000

	// FeeAddCredits conversion of ACME tokens to credits a "free" transaction
	FeeAddCredits Fee = 0

	// FeeUpdateKeyPage $0.03
	FeeUpdateKeyPage Fee = 300

	// FeeCreateScratchChain $0.25
	FeeCreateScratchChain Fee = 2500

	//FeeWriteScratchData $0.0001 / 256 bytes
	FeeWriteScratchData Fee = 1

	// FeeSignPending $0.001
	FeeSignPending Fee = 10
)

func ComputeFee(tx *Envelope) (Fee, error) {
	// Do not charge fees for the DN or BVNs
	if IsDnUrl(tx.Transaction.Origin) {
		return 0, nil
	}
	if _, ok := ParseBvnUrl(tx.Transaction.Origin); ok {
		return 0, nil
	}

	txType := tx.Transaction.Type()
	if txType == TransactionTypeUnknown {
		return 0, fmt.Errorf("cannot compute fee with no data defined for transaction")
	}
	switch TransactionType(txType) {
	case TransactionTypeCreateIdentity:
		return FeeCreateIdentity, nil
	case TransactionTypeCreateTokenAccount:
		return FeeCreateTokenAccount, nil
	case TransactionTypeSendTokens:
		return FeeSendTokens, nil
	case TransactionTypeCreateDataAccount:
		return FeeCreateDataAccount, nil
	case TransactionTypeWriteData:
		size := len(tx.Transaction.Body)
		if size > WriteDataMax {
			return 0, fmt.Errorf("data amount exceeds %v byte entry limit", WriteDataMax)
		}
		if size <= 0 {
			return 0, fmt.Errorf("insufficient data provided for %v needed to compute cost", txType)
		}
		return FeeWriteData * Fee(size/256+1), nil
	case TransactionTypeWriteDataTo:
		return FeeWriteDataTo, nil
	case TransactionTypeAcmeFaucet:
		return FeeAcmeFaucet, nil
	case TransactionTypeCreateToken:
		return FeeCreateToken, nil
	case TransactionTypeIssueTokens:
		return FeeIssueTokens, nil
	case TransactionTypeBurnTokens:
		return FeeBurnTokens, nil
	case TransactionTypeCreateKeyPage:
		return FeeCreateKeyPage, nil
	case TransactionTypeCreateKeyBook:
		return FeeCreateKeyBook, nil
	case TransactionTypeAddCredits:
		return FeeAddCredits, nil
	case TransactionTypeUpdateKeyPage:
		return FeeUpdateKeyPage, nil
	case TransactionTypeSignPending:
		return FeeSignPending, nil
	default:
		//by default assume if type isn't specified, there is no charge for tx
		return 0, nil
	}
}
