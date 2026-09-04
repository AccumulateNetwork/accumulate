// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package bcdb

import (
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
	require.Zero(t, d.FallbackWalks(), "a mutable miss must not walk history")
	require.Empty(t, d.DeepFallbacks())
}

// A miss on a PERMANENT shape still walks history through the fallback,
// because readers that legitimately reach past the window -- a pending
// transaction's message, dispatch after the anchor returns -- do not yet
// take a deep batch (DIFFERENCES E9).  The miss is counted so the soak
// can name those readers; the walk is counted so its removal shows.
func TestPermanentMissIsCountedAndStillWalks(t *testing.T) {
	d, err := Open(filepath.Join(t.TempDir(), "db"))
	require.NoError(t, err)
	defer func() { require.NoError(t, d.Close()) }()

	put(t, d, record.NewKey("Message", [32]byte{1}, "Main"), "message")

	missing := record.NewKey("Message", [32]byte{2}, "Main")
	require.True(t, isWriteOnce(missing))

	batch := d.Begin(nil, false)
	defer batch.Discard()
	_, err = batch.Get(missing)
	var nf *database.NotFoundError
	require.ErrorAs(t, err, &nf)

	require.Equal(t, uint64(1), d.ShallowMisses()[keyShape(missing)])
	require.Equal(t, uint64(1), d.FallbackWalks(), "a permanent miss walks, until its readers are deep")
	require.Empty(t, d.DeepFallbacks(), "nothing was there to find")
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
	_, err = batch.Get(record.NewKey("Message", [32]byte{2}, "Main"))
	var nf *database.NotFoundError
	require.ErrorAs(t, err, &nf)
	require.Empty(t, d.ShallowMisses())
	require.Zero(t, d.FallbackWalks())
}
