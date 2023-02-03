// Copyright 2023 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package messaging

import (
	"fmt"

	"gitlab.com/accumulatenetwork/accumulate/pkg/errors"
	"gitlab.com/accumulatenetwork/accumulate/pkg/url"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

func (e *Envelope) Normalize() ([]Message, error) {
	// Validate the envelope's TxHash
	var defaultTxID *url.TxID
	switch len(e.TxHash) {
	case 32:
		defaultTxID = protocol.UnknownUrl().WithTxID(*(*[32]byte)(e.TxHash))
	case 0:
		// Ok
	default:
		return nil, fmt.Errorf("invalid hash length: want 32, got %d", len(e.TxHash))
	}

	// Convert transactions to messages
	messages := e.Messages
	for _, txn := range e.Transaction {
		messages = append(messages, &UserTransaction{Transaction: txn})
	}

	// Determine which transactions have a signature
	unsigned := map[[32]byte]struct{}{}
	for i, msg := range e.Messages {
		switch msg := msg.(type) {
		case *UserTransaction:
			hash, err := getTxnHash(msg.Transaction, defaultTxID)
			if err != nil {
				return nil, errors.UnknownError.WithFormat("message %d: %w", i, err)
			}

			unsigned[*(*[32]byte)(hash)] = struct{}{}
		}
	}

	// If envelope.TransactionHash is unset and there's exactly one transaction,
	// use that transaction as the default to sign
	if defaultTxID == nil && len(unsigned) == 1 {
		for hash := range unsigned {
			defaultTxID = protocol.UnknownUrl().WithTxID(hash)
		}
	}

	// Convert signatures to messages
	for i, sig := range e.Signatures {
		switch {
		case sig.GetTransactionHash() != [32]byte{}:
			// Signature specifies the transaction hash
			messages = append(messages, &UserSignature{
				Signature: sig,
				TxID:      protocol.UnknownUrl().WithTxID(sig.GetTransactionHash()),
			})

		case defaultTxID != nil:
			// Use the default hash
			messages = append(messages, &UserSignature{
				Signature: sig,
				TxID:      defaultTxID,
			})

		default:
			return nil, errors.BadRequest.WithFormat("signature %d: missing hash", i)
		}

	}

	// A transaction with no signatures is invalid
	for _, msg := range messages {
		switch msg := msg.(type) {
		case *UserSignature:
			delete(unsigned, msg.TxID.Hash())
		case *ValidatorSignature:
			delete(unsigned, msg.Signature.GetTransactionHash())
		}
	}
	for hash := range unsigned {
		return nil, errors.BadRequest.WithFormat("transaction %X is not signed", hash[:4]) //nolint:rangevarref
	}

	return messages, nil
}

func getTxnHash(txn *protocol.Transaction, defaultTxID *url.TxID) ([]byte, error) {
	if txn.Body == nil {
		return nil, errors.BadRequest.With("nil body")
	}

	hash := txn.GetHash()
	switch {
	case len(hash) == 32:
		// Normal transaction or a remote transaction that includes a hash
		return hash, nil

	case defaultTxID != nil:
		// Envelope specifies the transaction hash
		h := defaultTxID.Hash()
		hash = h[:]

		// Set the remote transaction's hash
		if remote, ok := txn.Body.(*protocol.RemoteTransaction); ok {
			remote.Hash = defaultTxID.Hash()
		}

		return hash, nil

	default:
		// No hash
		return nil, errors.BadRequest.With("remote transaction: missing hash")
	}
}
