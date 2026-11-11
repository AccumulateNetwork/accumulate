// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package messaging

import (
	"testing"

	"github.com/stretchr/testify/require"
	"gitlab.com/accumulatenetwork/accumulate/internal/core/hash"
	"gitlab.com/accumulatenetwork/accumulate/pkg/database/merkle"
	"gitlab.com/accumulatenetwork/accumulate/pkg/url"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

func malformedSeq(inner Message) *SequencedMessage {
	return &SequencedMessage{
		Message:     inner,
		Source:      protocol.PartitionUrl("BVN1"),
		Destination: protocol.PartitionUrl("BVN0"),
		Number:      1,
	}
}

func malformedTxn() *TransactionMessage {
	return &TransactionMessage{Transaction: &protocol.Transaction{
		Header: protocol.TransactionHeader{Principal: protocol.AccountUrl("foo")},
		Body:   &protocol.SyntheticDepositTokens{},
	}}
}

// A message missing a field that naming it, routing it or executing it reads
// is refused by Normalize, before anything reads the field (#4572), whether it
// is submitted or carried in a block.
func TestNormalize_RefusesMalformedMessages(t *testing.T) {
	cases := map[string]Message{
		"nil message":                        nil,
		"transaction without transaction":    &TransactionMessage{},
		"signature without signature":        &SignatureMessage{},
		"credit payment without txid":        &CreditPayment{},
		"signature request without txid":     &SignatureRequest{},
		"anchor without anchor":              &BlockAnchor{},
		"anchor of a bare transaction":       &BlockAnchor{Anchor: &TransactionMessage{}},
		"anchor of an empty sequence":        &BlockAnchor{Anchor: malformedSeq(nil)},
		"anchor without body":                &BlockAnchor{Anchor: malformedSeq(&TransactionMessage{Transaction: &protocol.Transaction{}})},
		"sequenced without message":          malformedSeq(nil),
		"sequenced transaction without one":  malformedSeq(&TransactionMessage{}),
		"sequenced transaction no body":      malformedSeq(&TransactionMessage{Transaction: &protocol.Transaction{}}),
		"sequenced signature without one":    malformedSeq(&SignatureMessage{}),
		"sequenced signature without txid":   malformedSeq(&SignatureMessage{Signature: &protocol.ReceiptSignature{}}),
		"synthetic without message":          &SyntheticMessage{},
		"synthetic of an empty sequence":     &SyntheticMessage{Message: malformedSeq(nil)},
		"synthetic of an empty synthetic":    &SyntheticMessage{Message: malformedSeq(&SyntheticMessage{})},
		"synthetic credit payment no txid":   &SyntheticMessage{Message: malformedSeq(&CreditPayment{})},
		"bad synthetic without message":      &BadSyntheticMessage{},
		"bad synthetic of an empty sequence": &BadSyntheticMessage{Message: malformedSeq(nil)},
	}
	for name, msg := range cases {
		t.Run(name, func(t *testing.T) {
			env := &Envelope{Messages: []Message{msg}}
			require.NotPanics(t, func() {
				_, err := env.Normalize()
				require.Error(t, err)
			})
		})
	}

	t.Run("nil transaction", func(t *testing.T) {
		env := &Envelope{Transaction: []*protocol.Transaction{nil}}
		require.NotPanics(t, func() {
			_, err := env.Normalize()
			require.Error(t, err)
		})
	})
	t.Run("nil signature", func(t *testing.T) {
		env := &Envelope{Signatures: []protocol.Signature{nil}}
		require.NotPanics(t, func() {
			_, err := env.Normalize()
			require.Error(t, err)
		})
	})
}

// A well-formed synthetic message still normalizes: the check refuses only
// what is missing.
func TestNormalize_AcceptsWellFormedSynthetic(t *testing.T) {
	env := &Envelope{Messages: []Message{
		&SyntheticMessage{Message: malformedSeq(malformedTxn())},
		&BadSyntheticMessage{Message: malformedSeq(malformedTxn())},
	}}
	msgs, err := env.Normalize()
	require.NoError(t, err)
	require.Len(t, msgs, 2)
}

// A BadSyntheticMessage is named by its proof's anchor. Its hash and ID do
// not read through a proof that is missing or carries a receipt list and no
// receipt (#4572); such a message is still named, and refused.
func TestBadSyntheticMessage_HashWithoutProof(t *testing.T) {
	for name, proof := range map[string]*protocol.AnnotatedReceipt{
		"no proof":     nil,
		"no receipt":   {ReceiptList: &merkle.ReceiptList{}},
		"short anchor": {Receipt: &merkle.Receipt{Anchor: []byte{1, 2, 3}}},
	} {
		t.Run(name, func(t *testing.T) {
			msg := &BadSyntheticMessage{Message: malformedSeq(malformedTxn()), Proof: proof}
			require.NotPanics(t, func() {
				id := msg.ID()
				require.Equal(t, msg.Hash(), id.Hash())
			})
		})
	}

	// The hash of a message with a proof is what it was.
	anchor := [32]byte{9, 8, 7}
	msg := &BadSyntheticMessage{
		Message: malformedSeq(malformedTxn()),
		Proof:   &protocol.AnnotatedReceipt{Receipt: &merkle.Receipt{Anchor: anchor[:]}},
	}
	var h hash.Hasher
	h.AddHash2(msg.Message.Hash())
	h.AddHash(&anchor)
	require.Equal(t, *(*[32]byte)(h.MerkleHash()), msg.Hash())
}

// A message of every type with every field left out can still be named,
// hashed and asked what it produced, caused and is for (#4572). Validation
// and execution do all of that for a message they refuse.
func TestMalformedMessage_NamingDoesNotPanic(t *testing.T) {
	for typ := MessageType(1); typ < 256; typ++ {
		msg, err := NewMessage(typ)
		if err != nil {
			continue
		}
		t.Run(typ.String(), func(t *testing.T) {
			require.NotPanics(t, func() {
				require.NotNil(t, msg.ID())
				_ = msg.Hash()
				if m, ok := msg.(MessageWithProduced); ok {
					for _, id := range m.GetProduced() {
						require.NotNil(t, id)
					}
				}
				if m, ok := msg.(MessageWithCauses); ok {
					for _, id := range m.GetCauses() {
						require.NotNil(t, id)
					}
				}
				if m, ok := msg.(MessageForTransaction); ok {
					_ = m.GetTxID()
				}
			})
		})
	}
}

// A well-formed message is named as it always was: the nil checks change
// nothing for a message that has its fields.
func TestMalformedMessage_WellFormedIDsUnchanged(t *testing.T) {
	txn := malformedTxn()
	seq := malformedSeq(txn)
	require.Equal(t, txn.Transaction.ID(), txn.ID())
	require.Equal(t, *(*[32]byte)(txn.Transaction.GetHash()), txn.Hash())
	syn := &SyntheticMessage{Message: seq}
	require.Equal(t, seq.ID().Account().WithTxID(syn.Hash()), syn.ID())
	require.Equal(t, []*url.TxID{seq.ID()}, syn.GetProduced())
	cp := &CreditPayment{TxID: txn.ID()}
	require.Equal(t, txn.ID().Account().WithTxID(cp.Hash()), cp.ID())
	req := &SignatureRequest{Authority: protocol.AccountUrl("bar"), TxID: txn.ID(), Cause: txn.ID()}
	require.Equal(t, []*url.TxID{txn.ID()}, req.GetCauses())
	require.Equal(t, []*url.TxID{txn.ID()}, req.GetProduced())
	require.Equal(t, req.Authority.WithTxID(req.Hash()), req.ID())
	require.Equal(t, seq.Destination.WithTxID(seq.Hash()), seq.ID())
	rs := &SignatureMessage{Signature: &protocol.ReceiptSignature{SourceNetwork: protocol.DnUrl()}}
	require.Equal(t, protocol.DnUrl().WithTxID(rs.Hash()).String(), rs.ID().String())
}

// A message missing the account it is named by is named under acc://unknown,
// not with an ID that has no account: such an ID cannot be recorded (#4572).
func TestMalformedMessage_NamedWithoutAccount(t *testing.T) {
	seq := malformedSeq(malformedTxn())
	seq.Destination = nil
	for name, msg := range map[string]Message{
		"sequenced without destination":       seq,
		"signature request without authority": &SignatureRequest{},
		"receipt signature without source":    &SignatureMessage{Signature: &protocol.ReceiptSignature{}},
	} {
		t.Run(name, func(t *testing.T) {
			require.NotPanics(t, func() {
				require.True(t, protocol.UnknownUrl().Equal(msg.ID().Account()))
				_ = msg.ID().String()
			})
		})
	}
}
