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
	c := New(0)
	bvn1, bvn2 := protocol.PartitionUrl("BVN1"), protocol.PartitionUrl("BVN2")
	tx := c.Begin(3)
	for n := uint64(1); n <= 5; n++ {
		tx.AddAnchor(n, n, &protocol.Transaction{Body: &protocol.DirectoryAnchor{PartitionAnchor: protocol.PartitionAnchor{MinorBlockIndex: n}}})
	}
	tx.Commit()
	require.Equal(t, 5, c.AnchorLen())

	// One of two destinations has executed through 4: nothing goes
	tx = c.Begin(4)
	tx.ReleaseAnchors(bvn1, 4, 2)
	tx.Commit()
	require.Equal(t, 5, c.AnchorLen(), "the other destination has not spoken")

	// The other has executed through 2: anchors 1 and 2 go
	tx = c.Begin(5)
	tx.ReleaseAnchors(bvn2, 2, 2)
	tx.Commit()
	require.Equal(t, 3, c.AnchorLen())
	_, _, ok := c.PeekAnchor(2)
	require.False(t, ok)
	_, _, ok = c.PeekAnchor(3)
	require.True(t, ok)

	// A claim past what is held releases what is held and stops
	tx = c.Begin(6)
	tx.ReleaseAnchors(bvn1, ^uint64(0), 2)
	tx.ReleaseAnchors(bvn2, ^uint64(0), 2)
	tx.Commit()
	require.Zero(t, c.AnchorLen())

	// A single destination: a BVN's anchors go to the Directory alone
	tx = c.Begin(7)
	tx.AddAnchor(6, 7, &protocol.Transaction{Body: &protocol.BlockValidatorAnchor{}})
	tx.ReleaseAnchors(protocol.DnUrl(), 6, 1)
	tx.Commit()
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
	_, blocks = c.Len()
	require.Equal(t, 1, blocks, "every entry released: the block goes with them")
	_, ok = c.Block(3)
	require.False(t, ok)
}
