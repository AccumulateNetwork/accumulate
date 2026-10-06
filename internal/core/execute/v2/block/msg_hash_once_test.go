// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package block

import (
	"testing"

	"github.com/stretchr/testify/require"
	"gitlab.com/accumulatenetwork/accumulate/internal/database"
	"gitlab.com/accumulatenetwork/accumulate/pkg/errors"
	"gitlab.com/accumulatenetwork/accumulate/pkg/types/messaging"
	"gitlab.com/accumulatenetwork/accumulate/pkg/url"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

// countedAnchor is a block anchor that counts how often its hash and its ID
// are asked for.
type countedAnchor struct {
	*messaging.BlockAnchor
	hashes, ids int
}

func (c *countedAnchor) Hash() [32]byte {
	c.hashes++
	return c.BlockAnchor.Hash()
}

func (c *countedAnchor) ID() *url.TxID {
	c.ids++
	return c.BlockAnchor.ID()
}

func newCountedAnchor(number uint64) *countedAnchor {
	dn := protocol.DnUrl()
	txn := new(protocol.Transaction)
	txn.Header.Principal = dn.JoinPath(protocol.AnchorPool)
	txn.Body = &protocol.DirectoryAnchor{PartitionAnchor: protocol.PartitionAnchor{Source: dn, MinorBlockIndex: number}}
	return &countedAnchor{BlockAnchor: &messaging.BlockAnchor{
		Signature: &protocol.ED25519Signature{Signer: dn.JoinPath(protocol.Network), PublicKey: []byte{1}, Signature: []byte{2}},
		Anchor: &messaging.SequencedMessage{
			Message:     &messaging.TransactionMessage{Transaction: txn},
			Source:      dn,
			Destination: protocol.PartitionUrl("BVN1"),
			Number:      number,
		},
	}}
}

// Checking a message's status and recording it ask for the message's hash
// once, and its ID once. A block anchor's hash marshals the whole anchor it
// signs, at every level it is nested at, and these two functions asked for it
// six times and its ID three for every validator's signature on every anchor:
// 42 GiB an hour of allocation on a validator at ten transactions a second
// (#4542).
func TestAMessagesHashIsComputedOnceForItsContext(t *testing.T) {
	msg := newCountedAnchor(7)
	want, wantID := msg.BlockAnchor.Hash(), msg.BlockAnchor.ID()

	batch := database.OpenInMemory(nil).Begin(true)
	defer batch.Discard()
	ctx := &MessageContext{bundle: &bundle{Block: &Block{}}, message: msg}

	status, err := ctx.checkStatus(batch)
	require.NoError(t, err)
	require.True(t, status.TxID.Equal(wantID))
	require.NoError(t, ctx.recordMessageAndStatus(batch, status, errors.Delivered, nil))

	require.Equal(t, 1, msg.hashes, "the hash was computed again")
	require.Equal(t, 1, msg.ids, "the ID was computed again")

	// What was recorded is recorded under the message's hash.
	recorded, err := batch.Transaction2(want).Status().Get()
	require.NoError(t, err)
	require.Equal(t, errors.Delivered, recorded.Code)
	produced, err := batch.Transaction2(want).Produced().Get()
	require.NoError(t, err)
	require.Len(t, produced, 2, "a block anchor produces its signature and its anchor")
}

// The hash a context holds is its message's. A context given another message
// holds that message's hash, not the last one's.
func TestAContextGivenAnotherMessageHashesAgain(t *testing.T) {
	a, b := newCountedAnchor(7), newCountedAnchor(8)
	ctx := &MessageContext{message: a}
	require.Equal(t, a.BlockAnchor.Hash(), ctx.messageHash())
	require.True(t, a.BlockAnchor.ID().Equal(ctx.messageID()))

	ctx.message = b
	require.Equal(t, b.BlockAnchor.Hash(), ctx.messageHash())
	require.True(t, b.BlockAnchor.ID().Equal(ctx.messageID()))
	require.NotEqual(t, a.BlockAnchor.Hash(), b.BlockAnchor.Hash(), "precondition: two messages")
}
