package protocol

import (
	"encoding"
	"fmt"
)

// Fee is the unit cost of a transaction.
type Fee uint64

func (n Fee) AsUInt64() uint64 {
	return uint64(n)
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

	// FeeWriteScratchData $0.0001 / 256 bytes
	FeeWriteScratchData Fee = 1

	// FeeSignature $0.001
	FeeSignature Fee = 10
)

func BaseTransactionFee(typ TransactionType) (Fee, error) {
	// TODO Handle scratch accounts
	switch typ {
	case TransactionTypeCreateIdentity:
		return FeeCreateIdentity, nil
	case TransactionTypeCreateTokenAccount:
		return FeeCreateTokenAccount, nil
	case TransactionTypeSendTokens:
		return FeeSendTokens, nil
	case TransactionTypeCreateDataAccount:
		return FeeCreateDataAccount, nil
	case TransactionTypeWriteData:
		return FeeWriteData, nil
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
	case TransactionTypeUpdateManager, TransactionTypeRemoveManager:
		// TODO Fee schedule for these transactions
		return 0, nil
	case TransactionTypeSignPending:
		return FeeSignature, nil
	default:
		// All user transactions must have a defined fee amount, even if it's zero
		return 0, fmt.Errorf("unknown transaction type: %v", typ)
	}
}

func dataCount(obj encoding.BinaryMarshaler) (int, int, error) {
	// Check the transaction size (including signatures)
	data, err := obj.MarshalBinary()
	if err != nil {
		return 0, 0, err
	}

	// count the number of 256-byte chunks
	size := len(data)
	count := size / 256
	if size%256 != 0 {
		count++
	}

	return count, size, nil
}

func ComputeSignatureFee(sig Signature) (Fee, error) {
	// Check the transaction size
	count, size, err := dataCount(sig)
	if err != nil {
		return 0, err
	}
	if size > SignatureSizeMax {
		return 0, fmt.Errorf("signature size exceeds %v byte entry limit", SignatureSizeMax)
	}

	// If the signature is larger than 256 B, charge extra
	return FeeSignature * Fee(count), nil
}

func ComputeTransactionFee(tx *Envelope) (Fee, error) {
	// Do not charge fees for the DN or BVNs
	if IsDnUrl(tx.Transaction.Header.Principal) {
		return 0, nil
	}
	if _, ok := ParseBvnUrl(tx.Transaction.Header.Principal); ok {
		return 0, nil
	}

	// Don't charge for synthetic and internal transactions
	if !tx.Transaction.Type().IsUser() {
		return 0, nil
	}

	fee, err := BaseTransactionFee(tx.Transaction.Type())
	if err != nil {
		return 0, err
	}

	// Check the transaction size
	count, size, err := dataCount(tx.Transaction)
	if err != nil {
		return 0, err
	}
	if size > TransactionSizeMax {
		return 0, fmt.Errorf("transaction size exceeds %v byte entry limit", TransactionSizeMax)
	}

	// Charge an extra data fee per 256 B past the initial 256 B
	return fee + FeeWriteData*Fee(count-1), nil
}
