// Copyright 2023 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package messaging

import (
	"fmt"

	"gitlab.com/accumulatenetwork/accumulate/pkg/errors"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

// NormalizeLegacy normalizes the envelope into one or more legacy delivery
// messages.
func NormalizeLegacy(envelope *protocol.Envelope) ([]Message, error) {
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
	txnMap := make(map[[32]byte]*LegacyMessage, len(envelope.Transaction))
	txnList := make([]Message, 0, len(envelope.Transaction))
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

		delivery := new(LegacyMessage)
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

		// Get the existing delivery
		delivery, ok := txnMap[hash]
		if !ok {
			// Or create a new remote transaction
			body := new(protocol.RemoteTransaction)
			body.Hash = hash
			txn := new(protocol.Transaction)
			txn.Body = body
			delivery = new(LegacyMessage)
			delivery.Transaction = txn
			txnMap[hash] = delivery
			txnList = append(txnList, delivery)
		}

		// Add the signature to the delivery
		delivery.Signatures = append(delivery.Signatures, sig)
	}

	for _, delivery := range txnMap {
		// A transaction with no signatures is invalid
		if len(delivery.Signatures) == 0 {
			return nil, errors.BadRequest.WithFormat("the envelope does not contain any signatures matching transaction %X (%v)", delivery.Transaction.GetHash()[:8], delivery.Transaction.Body.Type())
		}
	}

	return txnList, nil
}
