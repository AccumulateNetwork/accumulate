package chain

import (
	"fmt"

	"github.com/AccumulateNetwork/accumulate/protocol"
	"github.com/AccumulateNetwork/accumulate/types"
	"github.com/AccumulateNetwork/accumulate/types/api/transactions"
	"github.com/AccumulateNetwork/accumulate/types/state"
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

	signatures := map[[32]byte][]byte{}
	for _, tx := range body.Transactions {
		signatures[tx.Transaction] = tx.Signature.Signature
	}

	unsigned := ledger.Synthetic.Unsigned
	ledger.Synthetic.Unsigned = make([][32]byte, 0, len(unsigned))

	for _, id := range unsigned {
		sig := signatures[id]
		if sig == nil {
			ledger.Synthetic.Unsigned = append(ledger.Synthetic.Unsigned, id)
			continue
		}

		// Load the pending transaction pending
		pending := new(state.PendingTransaction)
		err := st.LoadAs(id, pending)
		if err != nil {
			return err
		}

		// Add the signature
		pending.Signature = append(pending.Signature, &transactions.ED25519Sig{
			Nonce:     pending.TransactionState.SigInfo.Nonce,
			Signature: sig,
			PublicKey: tx.Signature[0].PublicKey,
		})

		// Validate it
		if !pending.Restore().ValidateSig() {
			return fmt.Errorf("invalid signature for txn %X", id)
		}

		// Write the signature
		st.SignTransaction(pending)

		// Send the transaction
		ledger.Synthetic.Unsent = append(ledger.Synthetic.Unsent, id)
	}

	st.Update(ledger)
	return nil
}
