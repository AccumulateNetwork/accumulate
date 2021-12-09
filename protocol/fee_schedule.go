package protocol

import (
	"encoding/binary"
	"fmt"

	"github.com/AccumulateNetwork/accumulate/types"
	"github.com/AccumulateNetwork/accumulate/types/api/transactions"
)

// Fee is the unit cost of a transaction.
type Fee int

func (n Fee) AsInt() int {
	return int(n)
}

// Fee Schedule
const (
	// FeeCreateIdentity $5.00 = 50000 credits @ 0.0001 / credit.
	FeeCreateIdentity Fee = 50000

	// FeeCreateTokenAccount $0.25
	FeeCreateTokenAccount Fee = 2500

	// FeeWithdrawTokens $0.03
	FeeWithdrawTokens Fee = 300

	// FeeCreateDataAccount $.25
	FeeCreateDataAccount Fee = 2500

	// FeeWriteData $0.001 / 256 bytes
	FeeWriteData Fee = 10

	// FeeWriteDataTo $0.001 / 256 bytes
	FeeWriteDataTo Fee = 10

	// FeeCreateToken $50.00
	FeeCreateToken Fee = 500000

	// FeeIssueTokens equiv. to token withdrawal @ $0.03
	FeeIssueTokens Fee = 300

	// FeeAcmeFaucet free
	FeeAcmeFaucet Fee = 0

	// FeeBurnTokens equiv. to token withdrawal
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
)

func ComputeFee(tx *transactions.GenTransaction) (int, error) {
	txType, n := binary.Uvarint(tx.Transaction)
	if n == 0 {
		return 0, fmt.Errorf("cannot compute fee with no data defined for transaction")
	}
	switch types.TransactionType(txType) {
	case types.TxTypeCreateIdentity:
		return FeeCreateIdentity.AsInt(), nil
	case types.TxTypeCreateTokenAccount:
		return FeeCreateTokenAccount.AsInt(), nil
	case types.TxTypeWithdrawTokens:
		return FeeWithdrawTokens.AsInt(), nil
	case types.TxTypeCreateDataAccount:
		return FeeCreateDataAccount.AsInt(), nil
	case types.TxTypeWriteData:
		size := len(tx.Transaction) - n
		if size > WriteDataMax {
			return 0, fmt.Errorf("data amount exceeds %v byte entry limit", WriteDataMax)
		}
		if size <= 0 {
			return 0, fmt.Errorf("insufficient data provided for %v needed to compute cost", txType)
		}
		return FeeWriteData.AsInt() * (size/256 + 1), nil
	case types.TxTypeWriteDataTo:
		return FeeWriteDataTo.AsInt(), nil
	case types.TxTypeAcmeFaucet:
		return FeeAcmeFaucet.AsInt(), nil
	case types.TxTypeCreateToken:
		return FeeCreateToken.AsInt(), nil
	case types.TxTypeIssueTokens:
		return FeeIssueTokens.AsInt(), nil
	case types.TxTypeBurnTokens:
		return FeeBurnTokens.AsInt(), nil
	case types.TxTypeCreateKeyPage:
		return FeeCreateKeyPage.AsInt(), nil
	case types.TxTypeCreateKeyBook:
		return FeeCreateKeyBook.AsInt(), nil
	case types.TxTypeAddCredits:
		return FeeAddCredits.AsInt(), nil
	case types.TxTypeUpdateKeyPage:
		return FeeUpdateKeyPage.AsInt(), nil
	default:
		//by default assume if type isn't specified, there is no charge for tx
		return 0, nil
	}
}
