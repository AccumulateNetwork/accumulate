package chain

import (
	"fmt"

	"github.com/AccumulateNetwork/accumulate/protocol"
	"github.com/AccumulateNetwork/accumulate/types"
	"github.com/AccumulateNetwork/accumulate/types/api/transactions"
)

type InternalTransactionsSigned struct{}

func (InternalTransactionsSigned) Type() types.TxType { return types.TxTypeInternalTransactionsSigned }

func (InternalTransactionsSigned) Validate(st *StateManager, tx *transactions.GenTransaction) error {
	body := new(protocol.InternalTransactionsSigned)
	err := tx.As(body)
	if err != nil {
		return fmt.Errorf("invalid payload: %v", err)
	}

	ledger, ok := st.Origin.(*protocol.InternalLedger)
	if !ok {
		return fmt.Errorf("invalid origin record: want chain type %v, got %v", types.ChainTypeInternalLedger, st.Origin.Header().Type)
	}

	signatures := map[[32]byte]*transactions.ED25519Sig{}
	for _, tx := range body.Transactions {
		signatures[tx.Transaction] = tx.Signature
	}

	unsigned := ledger.Synthetic.Unsigned
	ledger.Synthetic.Unsigned = make([][32]byte, 0, len(unsigned))

	for _, id := range unsigned {
		// Make a new variable to avoid the evil that is taking a pointer to a
		// loop variable
		id := id

		sig := signatures[id]
		if sig == nil {
			ledger.Synthetic.Unsigned = append(ledger.Synthetic.Unsigned, id)
			continue
		}

		// Load the transaction
		txState, err := st.GetTxnState(id)
		if err != nil {
			return err
		}

		// Add the signature
		gtx := txState.Restore()
		gtx.Signature = []*transactions.ED25519Sig{sig}

		// Validate it
		if !gtx.ValidateSig() {
			return fmt.Errorf("invalid signature for txn %X", id)
		}

		// Write the signature
		st.SignTransaction(id[:], sig)

		// Send the transaction
		ledger.Synthetic.Unsent = append(ledger.Synthetic.Unsent, id)
	}

	st.Update(ledger)
	return nil
}
