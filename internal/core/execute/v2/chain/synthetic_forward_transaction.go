// Copyright 2023 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package chain

import (
	"fmt"

	"gitlab.com/accumulatenetwork/accumulate/pkg/types/messaging"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

type SyntheticForwardTransaction struct{}

var _ PrincipalValidator = (*SyntheticForwardTransaction)(nil)

func (SyntheticForwardTransaction) Type() protocol.TransactionType {
	return protocol.TransactionTypeSyntheticForwardTransaction
}

func (SyntheticForwardTransaction) AllowMissingPrincipal(transaction *protocol.Transaction) bool {
	// This will be checked again when the additional transaction is executed.
	return true
}

func (SyntheticForwardTransaction) Execute(st *StateManager, tx *Delivery) (protocol.TransactionResult, error) {
	return (SyntheticForwardTransaction{}).Validate(st, tx)
}

func (SyntheticForwardTransaction) Validate(st *StateManager, tx *Delivery) (protocol.TransactionResult, error) {
	body, ok := tx.Transaction.Body.(*protocol.SyntheticForwardTransaction)
	if !ok {
		return nil, fmt.Errorf("invalid payload: want %T, got %T", new(protocol.SyntheticForwardTransaction), tx.Transaction.Body)
	}

	// Submit the transaction for processing
	st.State.ProcessForwarded(&messaging.UserTransaction{Transaction: body.Transaction})
	for _, sig := range body.Signatures {
		sig := sig // See docs/developer/rangevarref.md
		st.State.ProcessForwarded(&messaging.UserSignature{
			Signature:       &sig,
			TransactionHash: body.Transaction.ID().Hash(),
		})
	}
	return nil, nil
}
