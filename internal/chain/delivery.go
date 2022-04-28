package chain

import (
	"fmt"

	"gitlab.com/accumulatenetwork/accumulate/internal/database"
	"gitlab.com/accumulatenetwork/accumulate/internal/errors"
	"gitlab.com/accumulatenetwork/accumulate/internal/url"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
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
			return nil, protocol.Errorf(protocol.ErrorCodeInvalidRequest, "transaction %X is unsigned", delivery.Transaction.GetHash())
		}
	}

	return txnList, nil
}

type Delivery struct {
	parent      *Delivery
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

func (d *Delivery) NewForwarded(fwd *protocol.SyntheticForwardTransaction) *Delivery {
	signatures := make([]protocol.Signature, len(fwd.Signatures))
	for i, sig := range fwd.Signatures {
		sig := sig // See docs/developer/rangevarref.md
		signatures[i] = &sig
	}

	return d.NewChild(fwd.Transaction, signatures)
}

func (d *Delivery) NewSyntheticReceipt(hash [32]byte, source *url.URL, receipt *protocol.Receipt) *Delivery {
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
		return nil, errors.Format(errors.StatusUnknown, "load transaction status: %w", err)

	case status.Delivered:
		// Transaction has already been delivered
		return status, errors.Format(errors.StatusDelivered, "transaction %X has been delivered", d.Transaction.GetHash()[:4])
	}

	// Ignore produced synthetic transactions
	if status.Remote && !status.Pending {
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
		return nil, errors.Format(errors.StatusUnknown, "load transaction: %w", err)
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

func (d *Delivery) LoadSyntheticMetadata(batch *database.Batch, status *protocol.TransactionStatus) error {
	// Get the sequence number from the first signature?
	if len(d.Signatures) > 0 {
		if signature, ok := d.Signatures[0].(*protocol.SyntheticSignature); ok {
			d.SequenceNumber = signature.SequenceNumber
			d.SourceNetwork = signature.SourceNetwork
			return nil
		}
	}

	// Load the initiator signature set
	sigset, err := batch.Transaction(d.Transaction.GetHash()).ReadSignatures(status.Initiator)
	if err != nil {
		return errors.Format(errors.StatusUnknown, "load transaction: load initiator: %w", err)
	}

	var sigHash []byte
	for _, e := range sigset.Entries() {
		if e.Type == protocol.SignatureTypeSynthetic {
			sigHash = e.SignatureHash[:]
			break
		}
	}
	if sigHash == nil {
		return errors.NotFound("load transaction: missing synthetic origin signature")
	}

	state, err := batch.Transaction(sigHash).GetState()
	if err != nil {
		return errors.Format(errors.StatusUnknown, "load transaction: load synthetic origin signature: %w", err)
	}

	signature, ok := state.Signature.(*protocol.SyntheticSignature)
	if !ok {
		return errors.Format(errors.StatusInternalError, "load transaction: synthetic origin signature record is invalid")
	}

	d.SequenceNumber = signature.SequenceNumber
	d.SourceNetwork = signature.SourceNetwork
	return nil
}
