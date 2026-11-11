// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package messaging_test

import (
	"testing"

	"github.com/stretchr/testify/require"
	"gitlab.com/accumulatenetwork/accumulate/internal/core/hash"
	"gitlab.com/accumulatenetwork/accumulate/pkg/database/merkle"
	"gitlab.com/accumulatenetwork/accumulate/pkg/types/messaging"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

// A message with every field left out can still be named, hashed and asked
// what it produced and caused (#4572). Validation and execution do all of
// that for a message they refuse, and a missing field there took the node
// down.
func TestMalformedMessage_NamingDoesNotPanic(t *testing.T) {
	for typ := messaging.MessageType(1); typ < 256; typ++ {
		msg, err := messaging.NewMessage(typ)
		if err != nil {
			continue
		}
		t.Run(typ.String(), func(t *testing.T) {
			require.NotPanics(t, func() {
				require.NotNil(t, msg.ID())
				_ = msg.Hash()
				if m, ok := msg.(messaging.MessageWithProduced); ok {
					for _, id := range m.GetProduced() {
						require.NotNil(t, id)
					}
				}
				if m, ok := msg.(messaging.MessageWithCauses); ok {
					for _, id := range m.GetCauses() {
						require.NotNil(t, id)
					}
				}
				if m, ok := msg.(messaging.MessageForTransaction); ok {
					_ = m.GetTxID()
				}
			})
		})
	}
}

// A deprecated synthetic message that has its proof hashes as it always did:
// the nil-safe hash changes nothing for a message nodes produce.
func TestBadSyntheticMessage_HashUnchanged(t *testing.T) {
	seq := &messaging.SequencedMessage{
		Message:     &messaging.TransactionMessage{Transaction: &protocol.Transaction{Body: &protocol.DirectoryAnchor{}}},
		Source:      protocol.DnUrl(),
		Destination: protocol.PartitionUrl("BVN0"),
		Number:      1,
	}
	anchor := make([]byte, 32)
	anchor[0] = 1
	msg := &messaging.BadSyntheticMessage{Message: seq, Proof: &protocol.AnnotatedReceipt{
		Receipt: &merkle.Receipt{Anchor: anchor},
	}}

	var h hash.Hasher
	h.AddHash2(seq.Hash())
	h.AddHash((*[32]byte)(anchor))
	require.Equal(t, *(*[32]byte)(h.MerkleHash()), msg.Hash())

	// Without a proof it is named apart from that message
	require.NotEqual(t, msg.Hash(), (&messaging.BadSyntheticMessage{Message: seq}).Hash())

	// An anchor longer than 32 bytes is hashed by its first 32, as it was
	long := append(append([]byte{}, anchor...), 0xFF)
	longMsg := &messaging.BadSyntheticMessage{Message: seq, Proof: &protocol.AnnotatedReceipt{
		Receipt: &merkle.Receipt{Anchor: long},
	}}
	require.Equal(t, msg.Hash(), longMsg.Hash())
}

// A well-formed message is named as it always was: the nil checks change
// nothing for a message that has its fields.
func TestMalformedMessage_WellFormedIDsUnchanged(t *testing.T) {
	txn := &messaging.TransactionMessage{Transaction: &protocol.Transaction{
		Header: protocol.TransactionHeader{Principal: protocol.AccountUrl("foo")},
		Body:   &protocol.SyntheticDepositTokens{},
	}}
	seq := &messaging.SequencedMessage{Message: txn, Source: protocol.PartitionUrl("BVN1"), Destination: protocol.PartitionUrl("BVN0"), Number: 1}
	require.Equal(t, txn.Transaction.ID().String(), txn.ID().String())
	require.Equal(t, *(*[32]byte)(txn.Transaction.GetHash()), txn.Hash())
	require.Equal(t, seq.Destination.WithTxID(seq.Hash()).String(), seq.ID().String())
	syn := &messaging.SyntheticMessage{Message: seq}
	require.Equal(t, seq.ID().Account().WithTxID(syn.Hash()).String(), syn.ID().String())
	cp := &messaging.CreditPayment{TxID: txn.ID()}
	require.Equal(t, txn.ID().Account().WithTxID(cp.Hash()).String(), cp.ID().String())
	req := &messaging.SignatureRequest{Authority: protocol.AccountUrl("bar"), TxID: txn.ID(), Cause: txn.ID()}
	require.Equal(t, req.Authority.WithTxID(req.Hash()).String(), req.ID().String())
	require.Len(t, req.GetCauses(), 1)
	require.Len(t, req.GetProduced(), 1)
	rs := &messaging.SignatureMessage{Signature: &protocol.ReceiptSignature{SourceNetwork: protocol.DnUrl()}}
	require.Equal(t, protocol.DnUrl().WithTxID(rs.Hash()).String(), rs.ID().String())
}

// A message missing the account it is named by is named under acc://unknown,
// not with an ID that has no account: such an ID cannot be recorded (#4572).
func TestMalformedMessage_NamedWithoutAccount(t *testing.T) {
	seq := &messaging.SequencedMessage{Message: &messaging.TransactionMessage{Transaction: &protocol.Transaction{
		Header: protocol.TransactionHeader{Principal: protocol.AccountUrl("foo")},
		Body:   &protocol.SyntheticDepositTokens{},
	}}, Number: 1}
	for name, msg := range map[string]messaging.Message{
		"sequenced without destination":       seq,
		"signature request without authority": &messaging.SignatureRequest{},
		"receipt signature without source":    &messaging.SignatureMessage{Signature: &protocol.ReceiptSignature{}},
	} {
		t.Run(name, func(t *testing.T) {
			require.NotPanics(t, func() {
				require.True(t, protocol.UnknownUrl().Equal(msg.ID().Account()))
				_ = msg.ID().String()
			})
		})
	}
}

// A message missing a field that naming, routing or executing it reads is
// refused by Normalize, before anything reads the field (#4572), whether it
// is submitted or carried in a block.
func TestNormalize_RefusesMalformedMessages(t *testing.T) {
	seq := func(inner messaging.Message) *messaging.SequencedMessage {
		return &messaging.SequencedMessage{Message: inner, Source: protocol.PartitionUrl("BVN1"), Destination: protocol.PartitionUrl("BVN0"), Number: 1}
	}
	cases := map[string]messaging.Message{
		"nil message":                        nil,
		"transaction without transaction":    &messaging.TransactionMessage{},
		"signature without signature":        &messaging.SignatureMessage{},
		"anchor without anchor":              &messaging.BlockAnchor{},
		"anchor of a bare transaction":       &messaging.BlockAnchor{Anchor: &messaging.TransactionMessage{}},
		"anchor of an empty sequence":        &messaging.BlockAnchor{Anchor: seq(nil)},
		"anchor without body":                &messaging.BlockAnchor{Anchor: seq(&messaging.TransactionMessage{Transaction: &protocol.Transaction{}})},
		"sequenced without message":          seq(nil),
		"sequenced transaction without one":  seq(&messaging.TransactionMessage{}),
		"sequenced transaction no body":      seq(&messaging.TransactionMessage{Transaction: &protocol.Transaction{}}),
		"sequenced signature without one":    seq(&messaging.SignatureMessage{}),
		"synthetic without message":          &messaging.SyntheticMessage{},
		"synthetic of an empty sequence":     &messaging.SyntheticMessage{Message: seq(nil)},
		"synthetic of an empty synthetic":    &messaging.SyntheticMessage{Message: seq(&messaging.SyntheticMessage{})},
		"bad synthetic without message":      &messaging.BadSyntheticMessage{},
		"bad synthetic of an empty sequence": &messaging.BadSyntheticMessage{Message: seq(nil)},
	}
	for name, msg := range cases {
		t.Run(name, func(t *testing.T) {
			env := &messaging.Envelope{Messages: []messaging.Message{msg}}
			require.NotPanics(t, func() {
				_, err := env.Normalize()
				require.Error(t, err)
			})
		})
	}
}
