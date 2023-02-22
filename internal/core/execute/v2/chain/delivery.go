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
	special := map[[32]byte]bool{}

	get := func(hash [32]byte) *Delivery {
		if i, ok := txnIndex[hash]; ok {
			return deliveries[i]
		}

		d := new(Delivery)
		d.Transaction = new(protocol.Transaction)
		d.Transaction.Body = &protocol.RemoteTransaction{Hash: hash}

		txnIndex[hash] = len(deliveries)
		deliveries = append(deliveries, d)
		return d
	}

	var process func(msg messaging.Message, seq *messaging.SequencedMessage) error
	process = func(msg messaging.Message, seq *messaging.SequencedMessage) error {
		switch msg := msg.(type) {
		case *messaging.SequencedMessage:
			return process(msg.Message, msg)

		case *messaging.UserTransaction:
			hash := *(*[32]byte)(msg.Transaction.GetHash())
			d := get(hash)
			d.Transaction = msg.Transaction
			if seq != nil {
				d.Sequence = seq
			}

		case *messaging.UserSignature:
			// For now don't validate auth signatures
			if msg.Signature.Type() == protocol.SignatureTypeAuthority {
				special[msg.TxID.Hash()] = true
				return nil
			}

			d := get(msg.TxID.Hash())
			d.Signatures = append(d.Signatures, msg.Signature)
			if seq != nil {
				d.Sequence = seq
			}

		case *messaging.BlockAnchor:
			seq, ok := msg.Anchor.(*messaging.SequencedMessage)
			if !ok {
				return errors.BadRequest.With("anchor must contain a sequenced transaction")
			}
			txn, ok := seq.Message.(*messaging.UserTransaction)
			if !ok {
				return errors.BadRequest.With("anchor must contain a sequenced transaction")
			}

			d := get(txn.Hash())
			d.Signatures = append(d.Signatures, msg.Signature)
			return process(msg.Anchor, seq)

		case messaging.MessageForTransaction:
			// For now don't validate signature requests
			special[msg.GetTxID().Hash()] = true
			return nil

		case interface{ Unwrap() messaging.Message }:
			return process(msg.Unwrap(), seq)

		default:
			return errors.BadRequest.WithFormat("unsupported message type %v", msg.Type())
		}
		return nil
	}

	for _, msg := range messages {
		err := process(msg, nil)
		if err != nil {
			return nil, err
		}
	}

	// Remove entries that only have a signature request or auth signature
	for i := 0; i < len(deliveries); i++ {
		d := deliveries[i]
		if len(d.Signatures) > 0 {
			continue
		}
		if !special[d.Transaction.ID().Hash()] {
			continue
		}
		deliveries = append(deliveries[:i], deliveries[i+1:]...)
		i--
	}

	for _, delivery := range deliveries {
		if delivery.Transaction.Body.Type().IsSynthetic() {
			continue
		}
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
