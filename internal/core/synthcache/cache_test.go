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
