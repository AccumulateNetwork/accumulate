// Copyright 2022 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package chain

import (
	"fmt"

	"gitlab.com/accumulatenetwork/accumulate/internal/database"
	"gitlab.com/accumulatenetwork/accumulate/internal/database/smt/managed"
	"gitlab.com/accumulatenetwork/accumulate/internal/errors"
	"gitlab.com/accumulatenetwork/accumulate/pkg/url"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

// NormalizeEnvelope normalizes the envelope into one or more deliveries.
func NormalizeEnvelope(envelope *protocol.Envelope) ([]*Delivery, error) {
	// Validate the envelope's TxHash
	var envTxHash *[32]byte
	switch len(envelope.TxHash) {
	case 32:
		envTxHash = (*[32]byte)(envelope.TxHash)
	case 0:
		// Ok
	default:
		return nil, fmt.Errorf("invalid hash length: want 32, got %d", len(envelope.TxHash))
	}

	// Create a map (and an ordered list) of all transactions
	txnMap := make(map[[32]byte]*Delivery, len(envelope.Transaction))
	txnList := make([]*Delivery, 0, len(envelope.Transaction))
	for i, txn := range envelope.Transaction {
		if txn.Body == nil {
			return nil, fmt.Errorf("transaction %d: nil body", i)
		}

		hash := txn.GetHash()
		switch {
		case len(hash) == 32:
			// Normal transaction or a remote transaction that includes a hash

		case envTxHash != nil:
			// Envelope specifies the transaction hash
			hash = (*envTxHash)[:]

			// Set the remote transaction's hash
			if remote, ok := txn.Body.(*protocol.RemoteTransaction); ok {
				remote.Hash = *(*[32]byte)(hash)
			}

		default:
			// No hash
			return nil, fmt.Errorf("transaction %d: remote transaction: missing hash", i)
		}

		delivery := new(Delivery)
		delivery.Transaction = txn
		txnMap[*(*[32]byte)(hash)] = delivery
		txnList = append(txnList, delivery)
	}

	// Map signatures to transactions
	for i, sig := range envelope.Signatures {
		hash := sig.GetTransactionHash()
		switch {
		case hash != [32]byte{}:
			// Signature specifies the transaction hash

		case envTxHash != nil:
			// Envelope specifies the transaction hash
			hash = *envTxHash

		case len(txnMap) == 1:
			// There's only one transaction
			for hash = range txnMap {
				break
			}

		default:
			return nil, fmt.Errorf("multi-transaction envelope: signature %d: missing hash", i)
		}

		// Add the signature to the existing delivery
		delivery := txnMap[hash]
		if delivery != nil {
			delivery.Signatures = append(delivery.Signatures, sig)
			continue
		}

		// Or create a new remote transaction
		body := new(protocol.RemoteTransaction)
		body.Hash = hash
		txn := new(protocol.Transaction)
		txn.Body = body
		delivery = new(Delivery)
		delivery.Transaction = txn
		delivery.Signatures = []protocol.Signature{sig}
		txnMap[hash] = delivery
		txnList = append(txnList, delivery)
	}

	for _, delivery := range txnList {
		// A transaction with no signatures is invalid
		if len(delivery.Signatures) == 0 {
			return nil, errors.Format(errors.StatusBadRequest, "the envelope does not contain any signatures matching transaction %X (%v)", delivery.Transaction.GetHash()[:8], delivery.Transaction.Body.Type())
		}
	}

	return txnList, nil
}

type Delivery struct {
	parent   *Delivery
	internal bool

	Signatures  []protocol.Signature
	Transaction *protocol.Transaction
	State       ProcessTransactionState

	// For synthetic transactions
	SequenceNumber uint64
	SourceNetwork  *url.URL
}

func (d *Delivery) NewChild(transaction *protocol.Transaction, signatures []protocol.Signature) *Delivery {
	e := new(Delivery)
	e.parent = d
	e.Transaction = transaction
	e.Signatures = signatures
	return e
}

func (d *Delivery) NewInternal(transaction *protocol.Transaction) *Delivery {
	if !d.Transaction.Body.Type().IsSystem() {
		panic("illegal attempt to produce an internal transaction outside of a system transaction")
	}

	sig := new(protocol.InternalSignature)
	sig.Cause = *(*[32]byte)(d.Transaction.GetHash())
	transaction.Header.Initiator = *(*[32]byte)(sig.Metadata().Hash())
	sig.TransactionHash = *(*[32]byte)(transaction.GetHash())

	e := new(Delivery)
	e.parent = d
	e.internal = true
	e.Transaction = transaction
	e.Signatures = []protocol.Signature{sig}

	return e
}

func (d *Delivery) NewForwarded(fwd *protocol.SyntheticForwardTransaction) *Delivery {
	signatures := make([]protocol.Signature, len(fwd.Signatures))
	for i, sig := range fwd.Signatures {
		sig := sig // See docs/developer/rangevarref.md
		signatures[i] = &sig
	}

	return d.NewChild(fwd.Transaction, signatures)
}

func (d *Delivery) NewSyntheticReceipt(hash [32]byte, source *url.URL, receipt *managed.Receipt) *Delivery {
	return d.NewChild(&protocol.Transaction{
		Body: &protocol.RemoteTransaction{
			Hash: hash,
		},
	}, []protocol.Signature{
		&protocol.ReceiptSignature{
			SourceNetwork:   source,
			Proof:           *receipt,
			TransactionHash: hash,
		},
	})
}

func (d *Delivery) NewSyntheticFromSequence(hash [32]byte) *Delivery {
	e := new(Delivery)
	e.parent = d
	e.Transaction = &protocol.Transaction{
		Body: &protocol.RemoteTransaction{
			Hash: hash,
		},
	}
	return e
}

func (d *Delivery) Parent() *Delivery {
	return d.parent
}

func (d *Delivery) WasProducedInternally() bool {
	return d.parent != nil && d.internal
}

// WasProducedByPushedUpdate returns true if the transaction was produced by an
// update pushed via an anchor from the directory network.
func (d *Delivery) WasProducedByPushedUpdate() bool {
	return d.parent != nil && d.internal && d.parent.Transaction.Body.Type() == protocol.TransactionTypeDirectoryAnchor
}

// IsForwarded returns true if the transaction was delivered within a
// SyntheticForwardedTransaction.
func (d *Delivery) IsForwarded() bool {
	return d.parent != nil && d.parent.Transaction.Body.Type() == protocol.TransactionTypeSyntheticForwardTransaction
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
			return nil, errors.Format(errors.StatusInternalError, "submitted transaction does not match the locally stored transaction")
		}

	case errors.Is(err, errors.StatusNotFound):
		if isRemote {
			// Remote transactions are only supported if the BVN has a local copy
			return nil, errors.Format(errors.StatusUnknownError, "load transaction: %w", err)
		}

		// The delivery includes the full transaction so it's ok that the
		// transaction does not (yet) exist locally

	default:
		// Unknown error
		return nil, errors.Format(errors.StatusUnknownError, "load transaction: %w", err)
	}

	// Check the transaction status
	status, err := record.GetStatus()
	switch {
	case err != nil:
		// Unknown error
		return nil, errors.Format(errors.StatusUnknownError, "load transaction status: %w", err)

	case status.Delivered():
		// Transaction has already been delivered
		return status, errors.Format(errors.StatusDelivered, "transaction %X (%v) has been delivered", d.Transaction.GetHash()[:4], d.Transaction.Body.Type())
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
		return errors.Format(errors.StatusInternalError, "synthetic transaction sequence number is missing")
	}

	// Get the sequence number from the status
	d.SequenceNumber = status.SequenceNumber
	d.SourceNetwork = status.SourceNetwork
	return nil
}
