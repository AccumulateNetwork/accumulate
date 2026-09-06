// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package bcdb

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/prometheus/client_golang/prometheus/testutil"
	"github.com/stretchr/testify/require"
	"gitlab.com/accumulatenetwork/accumulate/pkg/database"
	"gitlab.com/accumulatenetwork/accumulate/pkg/types/record"
)

// Clearing a set writes a zero-length value (values/set.go marshals an
// empty set to nothing), and every delivered transaction clears three of
// them. A set is a mutable shape: it is routed to the dynamic layer with
// or without a tombstone, so the tombstone needs no exception. Before
// #4235 every clear added one to d.dyna and the exceptions file, forever:
// 5.4 M entries an hour at 500 tps.
func TestClearingASetAddsNoException(t *testing.T) {
	d, err := Open(filepath.Join(t.TempDir(), "db"))
	require.NoError(t, err)
	defer func() { require.NoError(t, d.Close()) }()

	const n = 200
	for i := 0; i < n; i++ {
		var h [32]byte
		h[0], h[1] = byte(i), byte(i>>8)
		batch := d.Begin(nil, true)
		// The three sets a delivered transaction clears, as the record
		// model writes them: Put of an empty value, and a Delete.
		require.NoError(t, batch.Put(record.NewKey("Transaction", h, "Payments"), nil))
		require.NoError(t, batch.Put(record.NewKey("Transaction", h, "Votes"), []byte{}))
		require.NoError(t, batch.Delete(record.NewKey("Transaction", h, "Signatures", "signer")))
		require.NoError(t, batch.Commit())
	}

	require.Zero(t, d.Exceptions(), "a tombstone on a mutable shape is not an exception")
	require.Zero(t, testutil.ToFloat64(exceptionsGauge.WithLabelValues(d.metricLabel)))
	_, err = os.Stat(d.exceptionsPath)
	require.ErrorIs(t, err, os.ErrNotExist, "nothing was appended to the exceptions file")

	// And they read as absent
	batch := d.Begin(nil, false)
	defer batch.Discard()
	_, err = batch.Get(record.NewKey("Transaction", [32]byte{}, "Payments"))
	var nf *database.NotFoundError
	require.ErrorAs(t, err, &nf)
}

// A tombstone over a key that WOULD route to the permanent layer is the
// case the exception exists for: the tombstone lands in the dynamic
// layer, which is read first, and a later write must follow it there or
// read as deleted while holding a value. That key is excepted, counted,
// persisted, and still reads as deleted across a restart.
func TestTombstoneOverAPermanentKeyIsExcepted(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "db")
	d, err := Open(dir)
	require.NoError(t, err)

	key := record.NewKey("Message", [32]byte{9}, "Main")
	require.True(t, isWriteOnce(key))
	put(t, d, key, "value")
	del(t, d, key)

	require.Equal(t, 1, d.Exceptions())
	require.Equal(t, 1.0, testutil.ToFloat64(exceptionsGauge.WithLabelValues(d.metricLabel)))

	assertAbsent := func(d *Database) {
		batch := d.Begin(nil, false)
		defer batch.Discard()
		_, err := batch.Get(key)
		var nf *database.NotFoundError
		require.ErrorAs(t, err, &nf, "the tombstone is found first")
	}
	assertAbsent(d)

	// The exception outlives the process; loading it is what the gauge
	// makes visible.
	d = reopen(t, dir, d)
	defer func() { require.NoError(t, d.Close()) }()
	require.Equal(t, 1, d.Exceptions(), "loaded from the exceptions file")
	require.Equal(t, 1.0, testutil.ToFloat64(exceptionsGauge.WithLabelValues(d.metricLabel)))
	assertAbsent(d)

	// And a write after the tombstone is not shadowed by it
	put(t, d, key, "again")
	require.Equal(t, "again", get(t, d, key))
	require.Equal(t, 1, d.Exceptions(), "the same key is one exception")
}
