// Copyright 2023 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package messaging

import (
	"crypto/sha256"

	"gitlab.com/accumulatenetwork/accumulate/internal/core/hash"
	"gitlab.com/accumulatenetwork/accumulate/pkg/errors"
	"gitlab.com/accumulatenetwork/accumulate/pkg/types/encoding"
	"gitlab.com/accumulatenetwork/accumulate/pkg/url"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

//go:generate go run gitlab.com/accumulatenetwork/accumulate/tools/cmd/gen-enum --package messaging enums.yml
//go:generate go run gitlab.com/accumulatenetwork/accumulate/tools/cmd/gen-types --package messaging messages.yml
//go:generate go run gitlab.com/accumulatenetwork/accumulate/tools/cmd/gen-types --package messaging --out unions_gen.go --language go-union messages.yml

// MessageType is the type of a [Message].
type MessageType int

// A Message is signature, transaction, or other message that may be sent to
// Accumulate to be processed.
type Message interface {
	encoding.UnionValue

	// ID is the ID of the message.
	ID() *url.TxID

	// Type is the type of the message.
	Type() MessageType

	// Hash returns the hash of the message.
	Hash() [32]byte
}

// UnwrapAs returns the message as the given type, unwrapping any wrappers.
func UnwrapAs[T any](msg Message) (T, bool) {
	for {
		switch m := msg.(type) {
		case T:
			return m, true
		case interface{ Unwrap() Message }:
			msg = m.Unwrap()
		default:
			var z T
			return z, false
		}
	}
}

func (m *UserTransaction) ID() *url.TxID { return m.Transaction.ID() }

func (m *SequencedMessage) ID() *url.TxID {
	return m.Destination.WithTxID(m.Hash())
}

func (m *SyntheticMessage) ID() *url.TxID {
	return m.Message.ID().Account().WithTxID(m.Hash())
}

func (m *BlockAnchor) ID() *url.TxID {
	return m.Signature.GetSigner().WithTxID(*(*[32]byte)(m.Signature.Hash()))
}

func (m *UserSignature) ID() *url.TxID {
	hash := m.Hash()
	switch sig := m.Signature.(type) {
	case *protocol.ReceiptSignature:
		return sig.SourceNetwork.WithTxID(hash)
	case *protocol.InternalSignature:
		return protocol.UnknownUrl().WithTxID(hash)
	default:
		return sig.RoutingLocation().WithTxID(hash)
	}
}

func (m *SignatureRequest) ID() *url.TxID {
	return m.Authority.WithTxID(m.Hash())
}

func (m *CreditPayment) ID() *url.TxID {
	return m.Payer.WithTxID(m.Hash())
}

func (m *SyntheticMessage) Unwrap() Message { return m.Message }
func (m *SequencedMessage) Unwrap() Message { return m.Message }

type MessageWithTransaction interface {
	Message
	GetTransaction() *protocol.Transaction
}

func (m *UserTransaction) GetTransaction() *protocol.Transaction { return m.Transaction }

type MessageForTransaction interface {
	Message
	GetTxID() *url.TxID
}

func (m *UserSignature) GetTxID() *url.TxID    { return m.TxID }
func (m *SignatureRequest) GetTxID() *url.TxID { return m.TxID }
func (m *CreditPayment) GetTxID() *url.TxID    { return m.TxID }

func (m *BlockAnchor) GetTxID() *url.TxID {
	if seq, ok := m.Anchor.(*SequencedMessage); ok {
		return seq.Message.ID()
	}

	// Should never happen
	return m.Anchor.ID()
}

type MessageWithSignature interface {
	MessageForTransaction
	GetSignature() protocol.Signature
}

func (m *UserSignature) GetSignature() protocol.Signature { return m.Signature }
func (m *BlockAnchor) GetSignature() protocol.Signature   { return m.Signature }

func (m *UserTransaction) Hash() [32]byte {
	return *(*[32]byte)(m.Transaction.GetHash())
}

func (m *UserSignature) Hash() [32]byte {
	return *(*[32]byte)(m.Signature.Hash())
}

func (m *SyntheticMessage) Hash() [32]byte {
	var h hash.Hasher
	h.AddHash2(m.Message.Hash())
	h.AddHash((*[32]byte)(m.Proof.Receipt.Anchor))
	return *(*[32]byte)(h.MerkleHash())
}

func (m *SequencedMessage) Hash() [32]byte {
	var h hash.Hasher
	h.AddHash2(m.Message.Hash())
	h.AddUrl2(m.Source)
	h.AddUrl2(m.Destination)
	h.AddUint(m.Number)
	return *(*[32]byte)(h.MerkleHash())
}

func (m *BlockAnchor) Hash() [32]byte {
	var h hash.Hasher
	if m.Signature == nil {
		h.AddHash2([32]byte{})
	} else {
		h.AddHash((*[32]byte)(m.Signature.Hash()))
	}
	if m.Anchor == nil {
		h.AddHash2([32]byte{})
	} else {
		h.AddHash2(m.Anchor.Hash())
	}
	return *(*[32]byte)(h.MerkleHash())
}

func (m *SignatureRequest) Hash() [32]byte {
	// If this fails something is seriously wrong
	b, err := m.MarshalBinary()
	if err != nil {
		panic(errors.InternalError.WithFormat("marshaling signature request: %w", err))
	}
	return sha256.Sum256(b)
}

func (m *CreditPayment) Hash() [32]byte {
	// If this fails something is seriously wrong
	b, err := m.MarshalBinary()
	if err != nil {
		panic(errors.InternalError.WithFormat("marshaling signature request: %w", err))
	}
	return sha256.Sum256(b)
}
