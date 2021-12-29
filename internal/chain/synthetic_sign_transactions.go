package chain

import (
	"fmt"

	"github.com/AccumulateNetwork/accumulate/protocol"
	"github.com/AccumulateNetwork/accumulate/types"
	"github.com/AccumulateNetwork/accumulate/types/api/transactions"
)

type SyntheticSignTransactions struct{}

func (SyntheticSignTransactions) Type() types.TxType { return types.TxTypeSyntheticSignTransactions }

func (SyntheticSignTransactions) Validate(st *StateManager, tx *transactions.GenTransaction) error {
	body := new(protocol.SyntheticSignTransactions)
	err := tx.As(body)
	if err != nil {
		return fmt.Errorf("invalid payload: %v", err)
	}

	// Process the signatures
	for _, entry := range body.Transactions {
		// Load the pending transaction state
		state, err := st.GetTxnState(entry.Txid)
		if err != nil {
			return err
		}

		// Convert it into a transaction
		synTxn := state.Restore()

		// Add the signature
		synTxn.Signature = append(synTxn.Signature, &transactions.ED25519Sig{
			Nonce:     entry.Nonce,
			Signature: entry.Signature,
			PublicKey: tx.Signature[0].PublicKey,
		})

		// Validate it
		if !synTxn.ValidateSig() {
			return fmt.Errorf("invalid signature for txn %X", entry.Txid)
		}

		// Queue the transaction for sending next block
		st.AddSignature(tx.Signature[0].PublicKey, &entry)
	}

	return nil
}
