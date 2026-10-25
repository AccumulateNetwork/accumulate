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
}
