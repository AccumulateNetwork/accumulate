// Copyright 2023 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package messaging

import (
	"gitlab.com/accumulatenetwork/accumulate/internal/core/hash"
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

func (m *UserTransaction) ID() *url.TxID { return m.Transaction.ID() }

func (m *SequencedMessage) ID() *url.TxID {
	return m.Destination.WithTxID(m.Hash())
}

func (m *SyntheticMessage) ID() *url.TxID {
	return m.Message.ID().Account().WithTxID(m.Hash())
}

func (m *ValidatorSignature) ID() *url.TxID {
	return m.Signature.GetSigner().WithTxID(*(*[32]byte)(m.Signature.Hash()))
}

func (m *UserSignature) ID() *url.TxID {
	hash := *(*[32]byte)(m.Signature.Hash())
	switch sig := m.Signature.(type) {
	case *protocol.ReceiptSignature:
		return sig.SourceNetwork.WithTxID(hash)
	case *protocol.InternalSignature:
		return protocol.UnknownUrl().WithTxID(hash)
	default:
		return sig.RoutingLocation().WithTxID(hash)
	}
}

func (m *SyntheticMessage) Unwrap() Message { return m.Message }
func (m *SequencedMessage) Unwrap() Message { return m.Message }

type MessageWithTransaction interface {
	Message
	GetTransaction() *protocol.Transaction
}

func (m *UserTransaction) GetTransaction() *protocol.Transaction { return m.Transaction }

type MessageWithSignature interface {
	Message
	GetSignature() protocol.Signature
	GetTxID() *url.TxID
}

func (m *UserSignature) GetSignature() protocol.Signature      { return m.Signature }
func (m *ValidatorSignature) GetSignature() protocol.Signature { return m.Signature }
func (m *UserSignature) GetTxID() *url.TxID                    { return m.TxID }

func (m *ValidatorSignature) GetTxID() *url.TxID {
	return protocol.UnknownUrl().WithTxID(m.Signature.GetTransactionHash())
}

func (m *UserTransaction) Hash() [32]byte {
	return *(*[32]byte)(m.Transaction.GetHash())
}

func (m *UserSignature) Hash() [32]byte {
	var h hash.Hasher
	h.AddHash((*[32]byte)(m.Signature.Hash()))
	h.AddTxID(m.TxID)
	return *(*[32]byte)(h.MerkleHash())
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

func (m *ValidatorSignature) Hash() [32]byte {
	var h hash.Hasher
	h.AddHash((*[32]byte)(m.Signature.Hash()))
	h.AddUrl2(m.Source)
	return *(*[32]byte)(h.MerkleHash())
}
