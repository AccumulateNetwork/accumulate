// Copyright 2024 The Accumulate Authors
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
//go:generate go run gitlab.com/accumulatenetwork/accumulate/tools/cmd/gen-types --package messaging messages.yml --go-include ./pkg/types/record
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

func (m *TransactionMessage) ID() *url.TxID { return m.Transaction.ID() }

func (m *SequencedMessage) ID() *url.TxID {
	return m.Destination.WithTxID(m.Hash())
}

func (m *BadSyntheticMessage) ID() *url.TxID {
	return m.Message.ID().Account().WithTxID(m.Hash())
}

func (m *SyntheticMessage) ID() *url.TxID {
	return m.Message.ID().Account().WithTxID(m.Hash())
}

func (m *BlockAnchor) ID() *url.TxID {
	return m.Signature.GetSigner().WithTxID(m.Hash())
}

func (m *BlockAnchor) OldID() *url.TxID {
	return m.Signature.GetSigner().WithTxID(*(*[32]byte)(m.Signature.Hash()))
}

func (m *SignatureMessage) ID() *url.TxID {
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
	return m.TxID.Account().WithTxID(m.Hash())
}

func (m *BlockSummary) ID() *url.TxID {
	return protocol.PartitionUrl(m.Partition).WithTxID(m.Hash())
}

func (m *NetworkUpdate) ID() *url.TxID {
	return protocol.DnUrl().WithTxID(m.Hash())
}

func (m *BadSyntheticMessage) Unwrap() Message { return m.Message }
func (m *SyntheticMessage) Unwrap() Message    { return m.Message }
func (m *SequencedMessage) Unwrap() Message    { return m.Message }

type MessageWithTransaction interface {
	Message
	GetTransaction() *protocol.Transaction
}

func (m *TransactionMessage) GetTransaction() *protocol.Transaction { return m.Transaction }

type MessageForTransaction interface {
	Message
	GetTxID() *url.TxID
}

func (m *SignatureMessage) GetTxID() *url.TxID { return m.TxID }
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

func (m *SignatureMessage) GetSignature() protocol.Signature { return m.Signature }
func (m *BlockAnchor) GetSignature() protocol.Signature      { return m.Signature }

type MessageWithCauses interface {
	Message
	GetCauses() []*url.TxID
}

func (m *SignatureRequest) GetCauses() []*url.TxID { return []*url.TxID{m.Cause} }

type MessageWithProduced interface {
	Message
	GetProduced() []*url.TxID
}

func (m *SequencedMessage) GetProduced() []*url.TxID    { return []*url.TxID{m.Message.ID()} }
func (m *BadSyntheticMessage) GetProduced() []*url.TxID { return []*url.TxID{m.Message.ID()} }
func (m *SyntheticMessage) GetProduced() []*url.TxID    { return []*url.TxID{m.Message.ID()} }
func (m *SignatureRequest) GetProduced() []*url.TxID    { return []*url.TxID{m.TxID} }

func (m *BlockAnchor) GetProduced() []*url.TxID {
	id := m.Signature.GetSigner().WithTxID(*(*[32]byte)(m.Signature.Hash()))
	return []*url.TxID{id, m.Anchor.ID()}
}

func (m *TransactionMessage) Hash() [32]byte {
	// A transaction message must contain nothing besides the transaction, so
	// this is safe
	return *(*[32]byte)(m.Transaction.GetHash())
}

func (m *SignatureMessage) Hash() [32]byte {
	// A signature message must contain nothing besides the signature and
	// optionally a transaction hash, so this is safe
	return *(*[32]byte)(m.Signature.Hash())
}

func (m *BadSyntheticMessage) Hash() [32]byte {
	// This is unsafe and buggy, which is why this type is deprecated
	var h hash.Hasher
	h.AddHash2(m.Message.Hash())
	h.AddHash((*[32]byte)(m.Proof.Receipt.Anchor))
	return *(*[32]byte)(h.MerkleHash())
}

func (m *SequencedMessage) Hash() [32]byte { return encoding.Hash(m) }
func (m *BlockAnchor) Hash() [32]byte      { return encoding.Hash(m) }
func (m *SignatureRequest) Hash() [32]byte { return encoding.Hash(m) }
func (m *CreditPayment) Hash() [32]byte    { return encoding.Hash(m) }
func (m *BlockSummary) Hash() [32]byte     { return encoding.Hash(m) }
func (m *SyntheticMessage) Hash() [32]byte { return encoding.Hash(m) }
func (m *NetworkUpdate) Hash() [32]byte    { return encoding.Hash(m) }
