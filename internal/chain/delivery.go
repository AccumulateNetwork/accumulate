package chain

import (
	"fmt"

	"gitlab.com/accumulatenetwork/accumulate/internal/database"
	"gitlab.com/accumulatenetwork/accumulate/internal/errors"
	"gitlab.com/accumulatenetwork/accumulate/internal/url"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
	"gitlab.com/accumulatenetwork/accumulate/smt/managed"
	"gitlab.com/accumulatenetwork/accumulate/smt/storage"
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
	// Check the transaction status
	record := batch.Transaction(d.Transaction.GetHash())
	status, err := record.GetStatus()
	switch {
	case err != nil:
		// Unknown error
		return nil, errors.Format(errors.StatusUnknownError, "load transaction status: %w", err)

	case status.Delivered():
		// Transaction has already been delivered
		return status, errors.Format(errors.StatusDelivered, "transaction %X has been delivered", d.Transaction.GetHash()[:4])
	}

	// Ignore produced synthetic transactions
	if status.Remote() {
		return status, nil
	}

	// Load previous transaction state
	txState, err := record.GetState()
	if err == nil {
		// Loaded existing the transaction from the database
		d.Transaction = txState.Transaction
		return status, nil
	} else if !errors.Is(err, storage.ErrNotFound) {
		// Unknown error
		return nil, errors.Format(errors.StatusUnknownError, "load transaction: %w", err)
	}

	// Did the envelope include the full body?
	if d.Transaction.Body.Type() != protocol.TransactionTypeRemote {
		return status, nil
	}

	// If the remote transaction does not specify a principal, the transaction
	// must exist locally
	principal := d.Transaction.Header.Principal
	if principal == nil {
		return nil, errors.NotFound("load transaction: no principal: transaction not found")
	}

	// If any signature is local or forwarded, the transaction must exist
	// locally
	for _, signature := range d.Signatures {
		if signature.RoutingLocation().LocalTo(principal) {
			return nil, errors.NotFound("load transaction: local signature: transaction not found")
		}
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
