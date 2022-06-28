package protocol

import (
	"encoding"

	"gitlab.com/accumulatenetwork/accumulate/internal/errors"
)

// Fee is the unit cost of a transaction.
type Fee uint64

func (n Fee) AsUInt64() uint64 {
	return uint64(n)
}

// General fees
const (
	// FeeFailedMaximum $0.01
	FeeFailedMaximum Fee = 100

	// FeeSignature $0.0001
	FeeSignature Fee = 1

	// FeeData $0.001 / 256 bytes
	FeeData Fee = 10

	// FeeScratchData $0.0001 / 256 bytes
	FeeScratchData Fee = 1

	// FeeUpdateAuth $0.03
	FeeUpdateAuth Fee = 300
)

// Transaction-specific fees
const (
	// FeeCreateIdentity $5.00 = 50000 credits @ 0.0001 / credit.
	FeeCreateIdentity Fee = 50000

	// FeeCreateTokenAccount $0.25
	FeeCreateTokenAccount Fee = 2500

	// FeeSendTokens $0.03
	FeeSendTokens Fee = 300

	// FeeCreateDataAccount $.25
	FeeCreateDataAccount Fee = 2500

	// FeeCreateToken $50.00
	FeeCreateToken Fee = 500000

	// FeeIssueTokens equiv. to token send @ $0.03
	FeeIssueTokens Fee = 300

	// FeeBurnTokens equiv. to token send
	FeeBurnTokens Fee = 10

	// FeeCreateKeyPage $1.00
	FeeCreateKeyPage Fee = 10000

	// FeeCreateKeyBook $1.00
	FeeCreateKeyBook Fee = 10000

	// FeeCreateScratchChain $0.25
	FeeCreateScratchChain Fee = 2500

	// MinimumCreditPurchase $0.01
	MinimumCreditPurchase Fee = 100
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
	case TransactionTypeUpdateKeyPage:
		return FeeUpdateAuth, nil
	case TransactionTypeUpdateAccountAuth:
		return FeeUpdateAuth, nil
	case TransactionTypeUpdateKey:
		return FeeUpdateAuth, nil
	case TransactionTypeRemote:
		return FeeSignature, nil
	case TransactionTypeAcmeFaucet,
		TransactionTypeAddCredits,
		TransactionTypeWriteData,
		TransactionTypeWriteDataTo:
		return 0, nil
	default:
		// All user transactions must have a defined fee amount, even if it's zero
		return 0, errors.Format(errors.StatusInternalError, "unknown transaction type: %v", typ)
	}
}

func dataCount(obj encoding.BinaryMarshaler) (int, int, error) {
	// Check the transaction size (including signatures)
	data, err := obj.MarshalBinary()
	if err != nil {
		return 0, 0, errors.Wrap(errors.StatusInternalError, err)
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
		return 0, errors.Wrap(errors.StatusUnknownError, err)
	}
	if size > SignatureSizeMax {
		return 0, errors.Format(errors.StatusBadRequest, "signature size exceeds %v byte entry limit", SignatureSizeMax)
	}

	// If the signature is larger than 256 B, charge extra
	return FeeSignature * Fee(count), nil
}

func ComputeTransactionFee(tx *Transaction) (Fee, error) {
	// Do not charge fees for the DN or BVNs
	if IsDnUrl(tx.Header.Principal) {
		return 0, nil
	}
	if _, ok := ParsePartitionUrl(tx.Header.Principal); ok {
		return 0, nil
	}

	// Don't charge for synthetic and internal transactions
	if !tx.Body.Type().IsUser() {
		return 0, nil
	}

	fee, err := BaseTransactionFee(tx.Body.Type())
	if err != nil {
		return 0, errors.Wrap(errors.StatusUnknownError, err)
	}

	// Check the transaction size
	count, size, err := dataCount(tx)
	if err != nil {
		return 0, errors.Wrap(errors.StatusUnknownError, err)
	}
	if size > TransactionSizeMax {
		return 0, errors.Format(errors.StatusBadRequest, "transaction size exceeds %v byte entry limit", TransactionSizeMax)
	}

	// Write data transactions have a base fee equal to the network fee
	switch tx.Body.Type() {
	case TransactionTypeWriteData,
		TransactionTypeWriteDataTo:
		// Charge the network fee per 256 B
	default:
		// Charge the network fee per 256 B past the initia 256 B
		count--
	}

	// Charge 1/10 the data fee for scratch accounts
	dataRate := FeeData
	if body, ok := tx.Body.(*WriteData); ok && body.Scratch {
		dataRate = FeeScratchData
	}

	// Charge an extra data fee per 256 B past the initial 256 B
	fee += dataRate * Fee(count)

	// Charge double if updating the account state
	if body, ok := tx.Body.(*WriteData); ok && body.WriteToState {
		fee *= 2
	}

	return fee, nil
}
