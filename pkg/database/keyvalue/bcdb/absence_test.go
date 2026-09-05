// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package bcdb

import (
	"fmt"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
	"gitlab.com/accumulatenetwork/accumulate/pkg/database"
	"gitlab.com/accumulatenetwork/accumulate/pkg/types/record"
)

// A miss on a MUTABLE shape is answered by the dynamic layer and never
// walks the permanent history (database spec, "Duplicates are caught at
// entry"): a mutable key is routed to the dynamic layer without
// exception, so there is nothing in permanent history to find.
func TestMutableMissStopsAtTheDynamicLayer(t *testing.T) {
	d, err := Open(filepath.Join(t.TempDir(), "db"))
	require.NoError(t, err)
	defer func() { require.NoError(t, d.Close()) }()

	// Both layers hold something, so a walk would have segments to probe
	put(t, d, record.NewKey("Account", "alice", "Main"), "alice")
	put(t, d, record.NewKey("Message", [32]byte{1}, "Main"), "message")

	pending := record.NewKey("Account", "alice", "Pending")
	require.False(t, isWriteOnce(pending), "Pending is a set, which is mutable")

	batch := d.Begin(nil, false)
	defer batch.Discard()
	_, err = batch.Get(pending)
	var nf *database.NotFoundError
	require.ErrorAs(t, err, &nf)

	require.Equal(t, uint64(1), d.ShallowMisses()[keyShape(pending)], "the miss is counted by shape")
	require.Empty(t, d.HistoryReads(), "no deep read was made")
}

// A miss on a PERMANENT shape is the answer: the window is the protocol's
// horizon, nothing walks history for a shallow reader, and the miss is
// counted so a reader that should have been deep shows in the numbers.
func TestPermanentMissIsAbsent(t *testing.T) {
	d, err := Open(filepath.Join(t.TempDir(), "db"))
	require.NoError(t, err)
	defer func() { require.NoError(t, d.Close()) }()

	old := record.NewKey("Message", [32]byte{1}, "Main")
	put(t, d, old, "old")
	for i := 0; i < 3*int(d.MergeLag); i++ {
		put(t, d, record.NewKey("Account", fmt.Sprintf("a%d", i), "Main"), "x")
	}

	// Past the window, a shallow reader is told the key is absent even
	// though history has it -- that is the contract -- and the miss counts
	batch := d.Begin(nil, false)
	defer batch.Discard()
	_, err = batch.Get(old)
	var nf *database.NotFoundError
	require.ErrorAs(t, err, &nf)
	require.Equal(t, uint64(1), d.ShallowMisses()[keyShape(old)])
	require.Empty(t, d.HistoryReads(), "nothing walked")

	// A deep reader finds it
	deep := d.BeginDeep(nil, false)
	defer deep.Discard()
	v, err := deep.Get(old)
	require.NoError(t, err)
	require.Equal(t, "old", string(v))
}

// A DEEP reader's miss is the reader's own business: it asked for
// history and is not a shallow miss.
func TestDeepReaderMissIsNotAShallowMiss(t *testing.T) {
	d, err := Open(filepath.Join(t.TempDir(), "db"))
	require.NoError(t, err)
	defer func() { require.NoError(t, d.Close()) }()

	put(t, d, record.NewKey("Message", [32]byte{1}, "Main"), "message")

	batch := d.BeginDeep(nil, false)
	defer batch.Discard()
	missing := record.NewKey("Message", [32]byte{2}, "Main")
	_, err = batch.Get(missing)
	var nf *database.NotFoundError
	require.ErrorAs(t, err, &nf)
	require.Empty(t, d.ShallowMisses())
	require.Equal(t, uint64(1), d.HistoryReads()[keyShape(missing)].Misses, "a deep miss is a deep read that found nothing")
}

// A deep reader's reads are attributed: shape, hit, whether the key was
// asked before, and who asked.  Two reads of one key are two hits and one distinct key
// -- the pattern a cache would serve; one read each of many keys is the
// pattern where the reader should carry the record instead.
func TestHistoryReadsAreAttributed(t *testing.T) {
	d, err := Open(filepath.Join(t.TempDir(), "db"))
	require.NoError(t, err)
	defer func() { require.NoError(t, d.Close()) }()

	old := record.NewKey("Message", [32]byte{1}, "Main")
	put(t, d, old, "old")

	// Roll the window past it: the filters cover N..2N commits
	for i := 0; i < 3*int(d.MergeLag); i++ {
		put(t, d, record.NewKey("Account", fmt.Sprintf("a%d", i), "Main"), "x")
	}

	getDeep := func() string {
		batch := d.BeginDeep(nil, false)
		defer batch.Discard()
		v, err := batch.Get(old)
		require.NoError(t, err)
		return string(v)
	}
	require.Equal(t, "old", getDeep())
	require.Equal(t, "old", getDeep())

	hr := d.HistoryReads()
	shape := keyShape(old)
	require.Contains(t, hr, shape, "the read reached history and was attributed")
	require.Equal(t, uint64(2), hr[shape].Hits)
	require.Equal(t, 1, hr[shape].Distinct, "one key, asked twice")
	require.Zero(t, hr[shape].Misses)
	require.NotEmpty(t, hr[shape].HitCallers, "the first read is sampled")
	require.Empty(t, hr[shape].MissCallers)

	// A deep miss is a deep read that found nothing
	batch := d.BeginDeep(nil, false)
	_, err = batch.Get(record.NewKey("Message", [32]byte{9}, "Main"))
	batch.Discard()
	require.Error(t, err)
	require.Equal(t, uint64(1), d.HistoryReads()[shape].Misses)
}
