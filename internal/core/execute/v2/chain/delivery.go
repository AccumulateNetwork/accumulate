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

	for _, msg := range messages {
		switch msg := msg.(type) {
		case *messaging.UserTransaction:
			hash := *(*[32]byte)(msg.Transaction.GetHash())
			if i, ok := txnIndex[hash]; ok {
				deliveries[i].Transaction = msg.Transaction
			} else {
				txnIndex[hash] = len(deliveries)
				deliveries = append(deliveries, &Delivery{Transaction: msg.Transaction})
			}

		case *messaging.UserSignature:
			if i, ok := txnIndex[msg.TransactionHash]; ok {
				deliveries[i].Signatures = append(deliveries[i].Signatures, msg.Signature)
			} else {
				txnIndex[msg.TransactionHash] = len(deliveries)
				deliveries = append(deliveries, &Delivery{Signatures: []protocol.Signature{msg.Signature}})
			}

		default:
			return nil, errors.BadRequest.WithFormat("unsupported message type %v", msg.Type())
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
	SequenceNumber     uint64
	SourceNetwork      *url.URL
	DestinationNetwork *url.URL
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
	record := batch.Transaction(d.Transaction.GetHash())
	txState, err := record.GetState()
	switch {
	case err == nil:
		// Loaded existing the transaction from the database
		if isRemote {
			// Overwrite the transaction in the delivery
			d.Transaction = txState.Transaction
		} else if !txState.Transaction.Equal(d.Transaction) {
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
	status, err := record.GetStatus()
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

func (d *Delivery) LoadSyntheticMetadata(batch *database.Batch, typ protocol.TransactionType, status *protocol.TransactionStatus) error {
	if typ.IsUser() {
		return nil
	}
	switch typ {
	case protocol.TransactionTypeSystemGenesis, protocol.TransactionTypeSystemWriteData:
		return nil
	}

	// Get the sequence number from the first signature?
	if len(d.Signatures) > 0 {
		if signature, ok := d.Signatures[0].(*protocol.PartitionSignature); ok {
			d.SequenceNumber = signature.SequenceNumber
			d.SourceNetwork = signature.SourceNetwork
			return nil
		}
	}

	if status.SequenceNumber == 0 {
		return errors.InternalError.WithFormat("synthetic transaction sequence number is missing")
	}

	// Get the sequence number from the status
	d.SequenceNumber = status.SequenceNumber
	d.SourceNetwork = status.SourceNetwork
	return nil
}
