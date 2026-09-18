// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package synthcache

import (
	"testing"

	"github.com/stretchr/testify/require"
	"gitlab.com/accumulatenetwork/accumulate/pkg/database/merkle"
	"gitlab.com/accumulatenetwork/accumulate/pkg/types/messaging"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

// A produced anchor goes to every destination under one number, and every
// copy the destinations send back carries how far they have executed this
// partition's anchors. The anchor is released when the last of them has said
// so; a destination that has not spoken holds everything (#4232).
func TestCache_ReleaseAnchorsOnDelivered(t *testing.T) {
	c := New(8 * RejoinGrace) // the horizon must not be what drops them here
	bvn1, bvn2 := protocol.PartitionUrl("BVN1"), protocol.PartitionUrl("BVN2")
	tx := c.Begin(3)
	for n := uint64(1); n <= 5; n++ {
		tx.AddAnchor(n, n, &protocol.Transaction{Body: &protocol.DirectoryAnchor{PartitionAnchor: protocol.PartitionAnchor{MinorBlockIndex: n}}})
	}
	tx.Commit()
	require.Equal(t, 5, c.AnchorLen())

	// One destination has executed through 4: nothing goes until every
	// destination has spoken
	tx = c.Begin(4)
	tx.ReleaseAnchors(bvn1, 4, 2)
	tx.Commit()
	age(c, 4, RejoinGrace)
	require.Equal(t, 5, c.AnchorLen(), "the other destination has not spoken")

	// The other has executed through 2: anchors 1 and 2 go -- once the word
	// has waited out the grace a rejoining validator of either destination
	// pulls them in (#4290)
	b := uint64(5 + RejoinGrace)
	tx = c.Begin(b)
	tx.ReleaseAnchors(bvn2, 2, 2)
	tx.Commit()
	require.Equal(t, 5, c.AnchorLen(), "the word waits out the grace")
	age(c, b, RejoinGrace)
	require.Equal(t, 3, c.AnchorLen())
	_, _, ok := c.PeekAnchor(2)
	require.False(t, ok)
	_, _, ok = c.PeekAnchor(3)
	require.True(t, ok)

	b += RejoinGrace + 1
	tx = c.Begin(b)
	tx.ReleaseAnchors(bvn1, ^uint64(0), 2)
	tx.ReleaseAnchors(bvn2, ^uint64(0), 2)
	tx.Commit()
	age(c, b, RejoinGrace)
	require.Zero(t, c.AnchorLen())

	b += RejoinGrace + 1
	tx = c.Begin(b)
	tx.AddAnchor(6, b, &protocol.Transaction{Body: &protocol.BlockValidatorAnchor{}})
	tx.ReleaseAnchors(protocol.DnUrl(), 6, 1)
	tx.Commit()
	age(c, b, RejoinGrace)
	require.Zero(t, c.AnchorLen(), "one destination is the whole fan-out")
}

// A block whose every entry has been released is dropped, and a block that
// produced nothing keeps no Directory receipt when it is dispatched: neither
// has a reader (#4232, review finding 30). The empty block's header stays to
// the horizon, since dispatch looks it up.
func TestCache_EmptyBlocksAreDropped(t *testing.T) {
	c := New(0)
	bvn1 := protocol.PartitionUrl("BVN1")
	tx := c.Begin(3)
	blk := &Block{Index: 3, Streams: map[string]*Stream{streamKey(bvn1): {Destination: bvn1, Segment: &merkle.Segment{First: 0}}}}
	for n := uint64(1); n <= 2; n++ {
		seq := &messaging.SequencedMessage{Number: n, Source: protocol.PartitionUrl("BVN0"), Destination: bvn1}
		e := &Entry{Stream: bvn1, Number: n, Index: int64(n - 1), Block: 3, Hash: seq.Hash(), Seq: seq}
		tx.Add(e)
		blk.Entries = append(blk.Entries, e)
		blk.Streams[streamKey(bvn1)].Segment.Append(e.Hash[:])
	}
	tx.SetBlock(blk)
	tx.Commit()
	tx = c.Begin(4)
	tx.SetBlock(&Block{Index: 4, Streams: map[string]*Stream{}}) // produced nothing
	tx.Commit()
	_, blocks := c.Len()
	require.Equal(t, 2, blocks)

	receipt := &protocol.PartitionAnchorReceipt{RootChainReceipt: &merkle.Receipt{}}
	c.MarkDispatched(4, 10, receipt)
	b4, ok := c.Block(4)
	require.True(t, ok, "the header stays: dispatch looks it up")
	require.True(t, b4.Dispatched)
	require.Nil(t, b4.DirectoryReceipt, "nothing to prove, so no receipt is kept")
	c.MarkDispatched(3, 10, receipt)
	b3, ok := c.Block(3)
	require.True(t, ok, "a block with entries stays")
	require.Same(t, receipt, b3.DirectoryReceipt)

	tx = c.Begin(5)
	tx.Release(bvn1, 2)
	tx.Commit()
	age(c, 5, RejoinGrace)
	_, blocks = c.Len()
	require.Equal(t, 1, blocks, "every entry released: the block goes with them")
	_, ok = c.Block(3)
	require.False(t, ok)
}
