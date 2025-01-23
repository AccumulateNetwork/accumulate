// Copyright 2025 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package messaging

import (
	"bytes"
	"fmt"
	"sort"

	"gitlab.com/accumulatenetwork/accumulate/pkg/errors"
	"gitlab.com/accumulatenetwork/accumulate/pkg/url"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

// Normalize converts an envelope into a normalized bundle of messages.
//
//   - Every transaction and signature is converted into a message.
//   - If any transaction is not signed, the envelope is rejected.
//   - If any signature does _not_ specify a transaction hash, the envelope is
//     rejected unless the envelope specifies a default transaction hash or
//     contains exactly one transaction.
//   - If the transaction corresponding to a signature is not present, a
//     placeholder transaction is added.
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

	// Convert everything to messages
	messages := make([]Message, 0, len(e.Messages)+len(e.Transaction)+len(e.Signatures))
	messages = append(messages, e.Messages...)
	for _, txn := range e.Transaction {
		messages = append(messages, &TransactionMessage{Transaction: txn})
	}
	for _, sig := range e.Signatures {
		messages = append(messages, &SignatureMessage{Signature: sig})
	}

	// Collect a set of all transaction hashes
	unsigned := map[[32]byte]struct{}{}
	for i, msg := range messages {
		txn, ok := msg.(*TransactionMessage)
		if !ok {
			continue
		}
		hash, err := getTxnHash(txn.Transaction, defaultTxID)
		if err != nil {
			return nil, errors.UnknownError.WithFormat("message %d: %w", i, err)
		}

		unsigned[*(*[32]byte)(hash)] = struct{}{}
	}

	// If envelope.TransactionHash is unset and there's exactly one transaction,
	// use that transaction as the default to sign
	if defaultTxID == nil && len(unsigned) == 1 {
		for hash := range unsigned {
			defaultTxID = protocol.UnknownUrl().WithTxID(hash)
		}
	}

	// Ensure every signature hash a transaction hash and collect a set of all
	// signed transaction hashes
	missing := map[[32]byte]struct{}{}
	for i, msg := range messages {
		sig, ok := msg.(*SignatureMessage)
		if !ok {
			continue
		}

		switch {
		case sig.TxID != nil:
			// Message specifies the transaction ID
			missing[sig.TxID.Hash()] = struct{}{}

		case sig.Signature.GetTransactionHash() != [32]byte{}:
			// Signature specifies the transaction hash
			sig.TxID = protocol.UnknownUrl().WithTxID(sig.Signature.GetTransactionHash())
			missing[sig.Signature.GetTransactionHash()] = struct{}{}

		case defaultTxID != nil:
			// Use the default hash
			sig.TxID = defaultTxID
			missing[defaultTxID.Hash()] = struct{}{}

		default:
			return nil, errors.BadRequest.WithFormat("signature %d: missing hash", i)
		}
	}

	// Add a placeholder for any signed transactions that are not present
	for hash := range unsigned {
		delete(missing, hash)
	}
	for hash := range missing {
		messages = append(messages, &TransactionMessage{
			Transaction: &protocol.Transaction{
				Body: &protocol.RemoteTransaction{
					Hash: hash,
				},
			},
		})
	}

	// Check for unsigned transactions
	for _, msg := range messages {
		if msg, ok := UnwrapAs[MessageForTransaction](msg); ok {
			delete(unsigned, msg.GetTxID().Hash())
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

// Sort sorts the summary's record and state tree update lists.
func (s *BlockSummary) Sort() {
	sort.Slice(s.RecordUpdates, sortUpdates(s.RecordUpdates))
	sort.Slice(s.StateTreeUpdates, sortUpdates(s.StateTreeUpdates))
}

// Sort sorts the summary's record and state tree update lists.
func (s *BlockSummary) IsSorted() bool {
	return true &&
		sort.SliceIsSorted(s.RecordUpdates, sortUpdates(s.RecordUpdates)) &&
		sort.SliceIsSorted(s.StateTreeUpdates, sortUpdates(s.StateTreeUpdates))
}

func (u *RecordUpdate) keyHash() [32]byte    { return u.Key.Hash() }
func (u *StateTreeUpdate) keyHash() [32]byte { return u.Key.Hash() }

func sortUpdates[T interface{ keyHash() [32]byte }](l []T) func(i, j int) bool {
	return func(i, j int) bool {
		a, b := l[i].keyHash(), l[j].keyHash()
		return bytes.Compare(a[:], b[:]) < 0
	}
}
