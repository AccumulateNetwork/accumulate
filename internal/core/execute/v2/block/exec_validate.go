// Copyright 2023 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package block

import (
	"gitlab.com/accumulatenetwork/accumulate/internal/core/execute/v2/chain"
	"gitlab.com/accumulatenetwork/accumulate/internal/database"
	"gitlab.com/accumulatenetwork/accumulate/pkg/errors"
	"gitlab.com/accumulatenetwork/accumulate/pkg/types/messaging"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

// Validate converts the message to a delivery and validates it. Validate
// returns an error if the message is not a [message.LegacyMessage].
func (x *ExecutorV2) Validate(batch *database.Batch, messages []messaging.Message) ([]*protocol.TransactionStatus, error) {
	b := new(bundle)
	b.BlockV2 = &BlockV2{Executor: (*Executor)(x)}
	b.messages = messages

	deliveries, err := chain.DeliveriesFromMessages(messages)
	if err != nil {
		return nil, errors.UnknownError.Wrap(err)
	}

	st := make([]*protocol.TransactionStatus, len(deliveries))
	for i, delivery := range deliveries {
		st[i] = new(protocol.TransactionStatus)
		st[i].Result, err = (*Executor)(x).validateEnvelope(batch, delivery, (&MessageContext{
			bundle: b,
			message: &messaging.UserTransaction{
				Transaction: delivery.Transaction,
			},
		}).txnWith(delivery.Transaction))

		if err != nil {
			st[i].Set(err)
		}

		// Wait until after ValidateEnvelope, because the transaction may get
		// loaded by LoadTransaction
		st[i].TxID = delivery.Transaction.ID()
	}

	return st, nil
}
