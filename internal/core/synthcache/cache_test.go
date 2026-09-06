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
	"gitlab.com/accumulatenetwork/accumulate/pkg/url"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

func entry(block uint64, stream string, number uint64, index int64) *Entry {
	seq := &messaging.SequencedMessage{Number: number, Destination: url.MustParse(stream)}
	return &Entry{Stream: seq.Destination, Number: number, Index: index, Block: block, Hash: seq.Hash(), Seq: seq}
}

// Nothing is visible until the block commits, and a discarded block leaves
// nothing behind: the cache never holds an entry the chain does not.
func TestTxn_CommitAndDiscard(t *testing.T) {
	c := New(0)
	tx := c.Begin(7)
	e := entry(7, "acc://bvn-BVN1.acme", 3, 10)
	tx.Add(e)
	tx.SetBlock(&Block{Index: 7, Streams: map[string]*Stream{streamKey(e.Stream): {Destination: e.Stream, Segment: &merkle.Segment{First: 10}}}, Entries: []*Entry{e}})
	tx.AddAnchor(2, 6, &protocol.Transaction{})
	tx.AddReceived(&protocol.DirectoryAnchor{})

	_, ok := c.Entry(e.Stream, 3)
	require.False(t, ok, "uncommitted")
	_, ok = c.Block(7)
	require.False(t, ok)

	tx.Commit()
	got, ok := c.Entry(e.Stream, 3)
	require.True(t, ok)
	require.Same(t, e, got)
	got, ok = c.ByHash(e.Hash)
	require.True(t, ok)
	require.Same(t, e, got)
	b, ok := c.Block(7)
	require.True(t, ok)
	require.Equal(t, uint64(7), b.Index)
	_, _, ok = c.Anchor(2)
	require.True(t, ok)
	last, ok := c.LastAnchoredBlock()
	require.True(t, ok)
	require.Equal(t, uint64(6), last)
	require.Len(t, c.TakeReceived(), 1)
	require.Empty(t, c.TakeReceived(), "drained")

	tx2 := c.Begin(8)
	tx2.Add(entry(8, "acc://bvn-BVN1.acme", 4, 11))
	tx2.Discard()
	_, ok = c.Entry(e.Stream, 4)
	require.False(t, ok, "discarded block left nothing")
}

// The horizon clears whole blocks and everything they produced, and the
// newest block is never trimmed.
func TestTrim(t *testing.T) {
	c := New(3)
	for b := uint64(1); b <= 6; b++ {
		tx := c.Begin(b)
		e := entry(b, "acc://bvn-BVN1.acme", b, int64(b))
		tx.Add(e)
		tx.SetBlock(&Block{Index: b, Entries: []*Entry{e}})
		tx.AddAnchor(b, b-1, &protocol.Transaction{})
		tx.Commit()
	}
	entries, blocks := c.Len()
	require.Equal(t, 4, blocks, "the newest block and the horizon before it: 3,4,5,6")
	require.Equal(t, 4, entries)
	_, ok := c.Block(2)
	require.False(t, ok)
	_, ok = c.Block(6)
	require.True(t, ok)
	_, _, ok = c.Anchor(1)
	require.False(t, ok)
	_, _, ok = c.Anchor(6)
	require.True(t, ok)
}

// A destination's word — it has executed our stream through n — releases the
// entries at or below n and the block segments that held only them; entries
// above n and other destinations' entries stay.
func TestCache_ReleaseOnDelivered(t *testing.T) {
	c := New(0)
	bvn1 := protocol.PartitionUrl("BVN1")
	bvn2 := protocol.PartitionUrl("BVN2")
	mk := func(dst *url.URL, n uint64, block uint64) *Entry {
		seq := &messaging.SequencedMessage{Number: n, Source: protocol.PartitionUrl("BVN0"), Destination: dst}
		return &Entry{Stream: dst, Number: n, Index: int64(n - 1), Block: block, Hash: seq.Hash(), Seq: seq}
	}
	// block 7: BVN1 #1-#3 and BVN2 #1; block 8: BVN1 #4
	tx := c.Begin(7)
	blk := &Block{Index: 7, Streams: map[string]*Stream{
		streamKey(bvn1): {Destination: bvn1, Segment: &merkle.Segment{First: 0}},
		streamKey(bvn2): {Destination: bvn2, Segment: &merkle.Segment{First: 0}},
	}}
	for _, e := range []*Entry{mk(bvn1, 1, 7), mk(bvn1, 2, 7), mk(bvn1, 3, 7), mk(bvn2, 1, 7)} {
		tx.Add(e)
		blk.Entries = append(blk.Entries, e)
		blk.Streams[streamKey(e.Stream)].Segment.Append(e.Hash[:])
	}
	tx.SetBlock(blk)
	tx.Commit()
	tx = c.Begin(8)
	blk8 := &Block{Index: 8, Streams: map[string]*Stream{streamKey(bvn1): {Destination: bvn1, Segment: &merkle.Segment{First: 3}}}}
	e4 := mk(bvn1, 4, 8)
	tx.Add(e4)
	blk8.Entries = append(blk8.Entries, e4)
	blk8.Streams[streamKey(bvn1)].Segment.Append(e4.Hash[:])
	tx.SetBlock(blk8)
	tx.Commit()
	entries, _ := c.Len()
	require.Equal(t, 5, entries)

	// BVN1 says it executed through 3
	tx = c.Begin(9)
	tx.Release(bvn1, 3)
	tx.Commit()
	entries, _ = c.Len()
	require.Equal(t, 2, entries, "BVN1 #4 and BVN2 #1 remain")
	_, ok := c.Entry(bvn1, 3)
	require.False(t, ok)
	_, ok = c.Entry(bvn1, 4)
	require.True(t, ok)
	_, ok = c.Entry(bvn2, 1)
	require.True(t, ok)
	b7, ok := c.Block(7)
	require.True(t, ok)
	require.Nil(t, b7.Stream(bvn1), "block 7's BVN1 segment held only released entries")
	require.NotNil(t, b7.Stream(bvn2))
	require.Len(t, b7.Entries, 1)
	b8, _ := c.Block(8)
	require.NotNil(t, b8.Stream(bvn1), "block 8's segment still proves #4")

	// A lower or repeated word changes nothing
	tx = c.Begin(10)
	tx.Release(bvn1, 2)
	tx.Commit()
	entries, _ = c.Len()
	require.Equal(t, 2, entries)
}
