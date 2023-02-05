// Copyright 2023 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package chain

import (
	"gitlab.com/accumulatenetwork/accumulate/internal/database"
	"gitlab.com/accumulatenetwork/accumulate/pkg/errors"
	"gitlab.com/accumulatenetwork/accumulate/pkg/types/messaging"
	"gitlab.com/accumulatenetwork/accumulate/pkg/url"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

// NormalizeEnvelope normalizes the envelope into one or more deliveries.
func NormalizeEnvelope(envelope *messaging.Envelope) ([]*Delivery, error) {
	messages, err := envelope.Normalize()
	if err != nil {
		return nil, err
	}
	return DeliveriesFromMessages(messages)
}

// DeliveriesFromMessages converts a set of messages into a set of deliveries.
func DeliveriesFromMessages(messages []messaging.Message) ([]*Delivery, error) {
	var deliveries []*Delivery
	txnIndex := map[[32]byte]int{}

	var process func(msg messaging.Message) error
	process = func(msg messaging.Message) error {
		switch msg := msg.(type) {
		case *messaging.SequencedMessage:
			hash := msg.Message.Hash()
			if i, ok := txnIndex[hash]; ok {
				deliveries[i].Sequence = msg
			} else {
				txnIndex[hash] = len(deliveries)
				deliveries = append(deliveries, &Delivery{Sequence: msg})
			}
			return process(msg.Message)

		case *messaging.UserTransaction:
			hash := *(*[32]byte)(msg.Transaction.GetHash())
			if i, ok := txnIndex[hash]; ok {
				deliveries[i].Transaction = msg.Transaction
			} else {
				txnIndex[hash] = len(deliveries)
				deliveries = append(deliveries, &Delivery{Transaction: msg.Transaction})
			}

		case *messaging.UserSignature:
			if i, ok := txnIndex[msg.TxID.Hash()]; ok {
				deliveries[i].Signatures = append(deliveries[i].Signatures, msg.Signature)
			} else {
				txnIndex[msg.TxID.Hash()] = len(deliveries)
				deliveries = append(deliveries, &Delivery{Signatures: []protocol.Signature{msg.Signature}})
			}

		case *messaging.ValidatorSignature:
			if i, ok := txnIndex[msg.Signature.GetTransactionHash()]; ok {
				deliveries[i].Signatures = append(deliveries[i].Signatures, msg.Signature)
			} else {
				txnIndex[msg.Signature.GetTransactionHash()] = len(deliveries)
				deliveries = append(deliveries, &Delivery{Signatures: []protocol.Signature{msg.Signature}})
			}

		case interface{ Unwrap() messaging.Message }:
			return process(msg.Unwrap())

		default:
			return errors.BadRequest.WithFormat("unsupported message type %v", msg.Type())
		}
		return nil
	}

	for _, msg := range messages {
		err := process(msg)
		if err != nil {
			return nil, err
		}
	}

	for _, delivery := range deliveries {
		if len(delivery.Signatures) == 0 {
			return nil, errors.BadRequest.WithFormat("transaction %x has no signatures", delivery.Transaction.GetHash()[:4])
		}
	}
	return deliveries, nil
}

type Delivery struct {
	Internal  bool
	Forwarded bool

	Signatures  []protocol.Signature
	Transaction *protocol.Transaction
	State       ProcessTransactionState

	// For synthetic transactions
	Sequence *messaging.SequencedMessage
}

func (d *Delivery) WasProducedInternally() bool {
	return d.Internal
}

// IsForwarded returns true if the transaction was delivered within a
// SyntheticForwardedTransaction.
func (d *Delivery) IsForwarded() bool {
	return d.Forwarded
}

// LoadTransaction attempts to load the transaction from the database.
func (d *Delivery) LoadTransaction(batch *database.Batch) (*protocol.TransactionStatus, error) {
	// Load previous transaction state
	isRemote := d.Transaction.Body.Type() == protocol.TransactionTypeRemote
	var txn messaging.MessageWithTransaction
	err := batch.Message(d.Transaction.ID().Hash()).Main().GetAs(&txn)
	switch {
	case err == nil:
		// Loaded existing the transaction from the database
		if isRemote {
			// Overwrite the transaction in the delivery
			d.Transaction = txn.GetTransaction()
		} else if !txn.GetTransaction().Equal(d.Transaction) {
			// This should be impossible
			return nil, errors.InternalError.WithFormat("submitted transaction does not match the locally stored transaction")
		}

	case errors.Is(err, errors.NotFound):
		if isRemote {
			// Remote transactions are only supported if the BVN has a local copy
			return nil, errors.UnknownError.WithFormat("load transaction: %w", err)
		}

		// The delivery includes the full transaction so it's ok that the
		// transaction does not (yet) exist locally

	default:
		// Unknown error
		return nil, errors.UnknownError.WithFormat("load transaction: %w", err)
	}

	// Check the transaction status
	status, err := batch.Transaction(d.Transaction.GetHash()).GetStatus()
	switch {
	case err != nil:
		// Unknown error
		return nil, errors.UnknownError.WithFormat("load transaction status: %w", err)

	case status.Delivered():
		// Transaction has already been delivered
		return status, errors.Delivered.WithFormat("transaction %X (%v) has been delivered", d.Transaction.GetHash()[:4], d.Transaction.Body.Type())
	}

	return status, nil
}

// CLONE of the same function from block - TODO remove once chain and block
// packages are merged
func getSequence(batch *database.Batch, id *url.TxID) (*messaging.SequencedMessage, error) {
	causes, err := batch.Message(id.Hash()).Cause().Get()
	if err != nil {
		return nil, errors.UnknownError.WithFormat("load causes: %w", err)
	}

	for _, id := range causes {
		msg, err := batch.Message(id.Hash()).Main().Get()
		switch {
		case err == nil:
			// Ok
		case errors.Is(err, errors.NotFound):
			continue
		default:
			return nil, errors.UnknownError.WithFormat("load message: %w", err)
		}

		if seq, ok := msg.(*messaging.SequencedMessage); ok {
			return seq, nil
		}
	}

	return nil, errors.NotFound.WithFormat("no cause of %v is a sequenced message", id)
}
