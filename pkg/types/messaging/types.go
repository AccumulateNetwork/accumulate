// Copyright 2026 The Accumulate Authors
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

// A message missing the field it is named by is refused, but validation and
// execution still name it for its status. It is named under acc://unknown
// rather than dereferenced (#4572): computing the ID of a refused message took
// the node down. Nothing changes for a message that has the field.
func named(u *url.URL, hash [32]byte) *url.TxID {
	if u == nil {
		u = protocol.UnknownUrl()
	}
	return u.WithTxID(hash)
}

func (m *TransactionMessage) ID() *url.TxID {
	if m.Transaction == nil {
		return named(nil, m.Hash())
	}
	return m.Transaction.ID()
}

func (m *SequencedMessage) ID() *url.TxID {
	return named(m.Destination, m.Hash())
}

func (m *BadSyntheticMessage) ID() *url.TxID {
	return named(innerAccount(m.Message), m.Hash())
}

func (m *SyntheticMessage) ID() *url.TxID {
	return named(innerAccount(m.Message), m.Hash())
}

func innerAccount(msg Message) *url.URL {
	if msg == nil {
		return nil
	}
	return msg.ID().Account()
}

// ID is the signer's URL with the anchor's hash. An anchor without a
// signature is named under acc://unknown, as above: any peer can send one.
func (m *BlockAnchor) ID() *url.TxID {
	if m.Signature == nil {
		return named(nil, m.Hash())
	}
	return named(m.Signature.GetSigner(), m.Hash())
}

func (m *BlockAnchor) OldID() *url.TxID {
	if m.Signature == nil {
		return named(nil, m.Hash())
	}
	return named(m.Signature.GetSigner(), *(*[32]byte)(m.Signature.Hash()))
}

func (m *SignatureMessage) ID() *url.TxID {
	hash := m.Hash()
	switch sig := m.Signature.(type) {
	case nil:
		return named(nil, hash)
	case *protocol.ReceiptSignature:
		return named(sig.SourceNetwork, hash)
	case *protocol.InternalSignature:
		return protocol.UnknownUrl().WithTxID(hash)
	default:
		return named(sig.RoutingLocation(), hash)
	}
}

func (m *SignatureRequest) ID() *url.TxID {
	return named(m.Authority, m.Hash())
}

func (m *CreditPayment) ID() *url.TxID {
	if m.TxID == nil {
		return named(nil, m.Hash())
	}
	return named(m.TxID.Account(), m.Hash())
}

func (m *BlockSummary) ID() *url.TxID {
	return protocol.PartitionUrl(m.Partition).WithTxID(m.Hash())
}

func (m *NetworkUpdate) ID() *url.TxID {
	return protocol.DnUrl().WithTxID(m.Hash())
}

// ID identifies the proof by its own content. A SyntheticProof is not addressed
// to an account and is never recorded on its own — it exists to be resolved by
// the synthetic messages travelling with it — so the ID only has to be stable
// and unique, which the hash gives. The directory is used as the authority
// because a proof carries no partition of its own.
func (m *SyntheticProof) ID() *url.TxID {
	return protocol.DnUrl().WithTxID(m.Hash())
}

func (m *MakeMajorBlock) ID() *url.TxID {
	return protocol.DnUrl().WithTxID(m.Hash())
}

func (m *DidUpdateExecutorVersion) ID() *url.TxID {
	return protocol.PartitionUrl(m.Partition).WithTxID(m.Hash())
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
		if seq.Message == nil {
			return nil // Refused by validation (#4572)
		}
		return seq.Message.ID()
	}

	// Should never happen
	if m.Anchor == nil {
		return nil // Refused by validation (#4572)
	}
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

func (m *SignatureRequest) GetCauses() []*url.TxID {
	if m.Cause == nil {
		return nil // Refused; recorded without a cause (#4572)
	}
	return []*url.TxID{m.Cause}
}

type MessageWithProduced interface {
	Message
	GetProduced() []*url.TxID
}

// A refused message's status is recorded with what it produced; a field it
// is missing produces nothing rather than being dereferenced (#4572).

func (m *SequencedMessage) GetProduced() []*url.TxID    { return producedBy(m.Message) }
func (m *BadSyntheticMessage) GetProduced() []*url.TxID { return producedBy(m.Message) }
func (m *SyntheticMessage) GetProduced() []*url.TxID    { return producedBy(m.Message) }

func (m *SignatureRequest) GetProduced() []*url.TxID {
	if m.TxID == nil {
		return nil
	}
	return []*url.TxID{m.TxID}
}

func (m *BlockAnchor) GetProduced() []*url.TxID {
	var ids []*url.TxID
	if m.Signature != nil {
		ids = append(ids, m.Signature.GetSigner().WithTxID(*(*[32]byte)(m.Signature.Hash())))
	}
	ids = append(ids, producedBy(m.Anchor)...)
	return ids
}

func producedBy(msg Message) []*url.TxID {
	if msg == nil {
		return nil
	}
	return []*url.TxID{msg.ID()}
}

func (m *TransactionMessage) Hash() [32]byte {
	// A transaction message must contain nothing besides the transaction, so
	// this is safe. One without a transaction is refused; it hashes as
	// encoded rather than being dereferenced (#4572).
	if m.Transaction == nil {
		return encoding.Hash(m)
	}
	return *(*[32]byte)(m.Transaction.GetHash())
}

func (m *SignatureMessage) Hash() [32]byte {
	// A signature message must contain nothing besides the signature and
	// optionally a transaction hash, so this is safe. One without a signature
	// is refused; it hashes as encoded rather than being dereferenced (#4572).
	if m.Signature == nil {
		return encoding.Hash(m)
	}
	return *(*[32]byte)(m.Signature.Hash())
}

func (m *BadSyntheticMessage) Hash() [32]byte {
	// This is unsafe and buggy, which is why this type is deprecated
	var h hash.Hasher
	var msgHash [32]byte
	if m.Message != nil {
		msgHash = m.Message.Hash()
	}
	h.AddHash2(msgHash)

	// A message without a proof, a receipt or an anchor of at least 32 bytes
	// is refused, but it is still named, and naming it must not panic
	// (#4572). Its anchor is hashed as bytes, which a well-formed message's
	// never is, so it cannot take a well-formed message's name. An anchor of
	// 32 bytes or more is hashed by its first 32, as it always was.
	var anchor []byte
	if m.Proof != nil && m.Proof.Receipt != nil {
		anchor = m.Proof.Receipt.Anchor
	}
	if len(anchor) >= 32 {
		h.AddHash((*[32]byte)(anchor))
	} else {
		h.AddBytes(anchor)
	}
	return *(*[32]byte)(h.MerkleHash())
}

func (m *SequencedMessage) Hash() [32]byte         { return encoding.Hash(m) }
func (m *BlockAnchor) Hash() [32]byte              { return encoding.Hash(m) }
func (m *SignatureRequest) Hash() [32]byte         { return encoding.Hash(m) }
func (m *CreditPayment) Hash() [32]byte            { return encoding.Hash(m) }
func (m *BlockSummary) Hash() [32]byte             { return encoding.Hash(m) }
func (m *SyntheticMessage) Hash() [32]byte         { return encoding.Hash(m) }
func (m *NetworkUpdate) Hash() [32]byte            { return encoding.Hash(m) }
func (m *MakeMajorBlock) Hash() [32]byte           { return encoding.Hash(m) }
func (m *SyntheticProof) Hash() [32]byte           { return encoding.Hash(m) }
func (m *DidUpdateExecutorVersion) Hash() [32]byte { return encoding.Hash(m) }
