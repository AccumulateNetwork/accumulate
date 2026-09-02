// Copyright 2025 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package bcdb

import (
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
	"gitlab.com/accumulatenetwork/accumulate/pkg/types/record"
)

func put(t *testing.T, d *Database, key *record.Key, value string) {
	t.Helper()
	batch := d.Begin(nil, true)
	require.NoError(t, batch.Put(key, []byte(value)))
	require.NoError(t, batch.Commit())
}

func get(t *testing.T, d *Database, key *record.Key) string {
	t.Helper()
	batch := d.Begin(nil, false)
	defer batch.Discard()
	value, err := batch.Get(key)
	require.NoError(t, err)
	return string(value)
}

// TestMisroute rewrites a record the classification calls write-once,
// which is the case route.go exists to catch.
//
// The permanent layer refuses the write.  What has to happen then is
// that the refusal is recorded and the data is still correct: the value
// goes to the dynamic layer, every later write of that key goes there
// too -- the dynamic layer is read first, so leaving them to the
// permanent layer would let a stale value shadow the current one -- and
// the shape is named so the classification can be fixed.
func TestMisroute(t *testing.T) {
	d, err := Open(filepath.Join(t.TempDir(), "db"))
	require.NoError(t, err)
	defer func() { require.NoError(t, d.Close()) }()

	var hash [32]byte
	// Summary(H).Main rather than Message(H).Main: both are write-once, and
	// rewriting one with a DIFFERENT value is the misroute this exercises --
	// which for a message or an anchor would mean a broken protocol rather
	// than a misclassification.
	key := record.NewKey("Summary", hash, "Main")
	require.True(t, isWriteOnce(key), "the point of the test is that this is misclassified")

	put(t, d, key, "first")
	require.Equal(t, "first", get(t, d, key))

	perm, _ := d.Stats()
	require.Zero(t, perm.PutConflict)
	require.Equal(t, uint64(1), perm.PutNew)

	// Rewriting it is what the permanent layer will not do
	put(t, d, key, "second")
	require.Equal(t, "second", get(t, d, key))

	perm, _ = d.Stats()
	require.Equal(t, uint64(1), perm.PutConflict, "the store should have refused")
	require.Equal(t, uint64(1), d.Shapes()[keyShape(key)].Misrouted)

	// And once the key is in the dynamic layer it has to stay there,
	// or the permanent layer's copy of "first" would be shadowed by
	// the dynamic layer's copy of "second" while the current value is
	// "third"
	put(t, d, key, "third")
	require.Equal(t, "third", get(t, d, key))

	perm, _ = d.Stats()
	require.Equal(t, uint64(1), perm.PutConflict, "the second rewrite should not have reached the permanent layer")
}

// TestDeleteThenWrite deletes a write-once record and writes it again.
//
// A deletion is a mutation whatever the record is, so the tombstone
// goes to the dynamic layer -- and that makes the key a dynamic key
// from then on.  Sending the next write to the permanent layer would
// leave the tombstone in front of it, and the record would read as
// deleted while holding a value.
func TestDeleteThenWrite(t *testing.T) {
	d, err := Open(filepath.Join(t.TempDir(), "db"))
	require.NoError(t, err)
	defer func() { require.NoError(t, d.Close()) }()

	var hash [32]byte
	hash[0] = 2
	key := record.NewKey("Message", hash, "Main")

	put(t, d, key, "first")

	batch := d.Begin(nil, true)
	require.NoError(t, batch.Delete(key))
	require.NoError(t, batch.Commit())

	batch = d.Begin(nil, false)
	_, err = batch.Get(key)
	require.Error(t, err, "the record should read as absent")
	batch.Discard()

	put(t, d, key, "second")
	require.Equal(t, "second", get(t, d, key), "the tombstone must not shadow the new value")
}

// TestDeleteBeforeWrite deletes a write-once record that was never
// written, and then writes it.
//
// The tombstone is in the dynamic layer and the dynamic layer is read
// first, so the write has to go there too even though nothing in the
// permanent layer would have refused it.  Routing it to the permanent
// layer leaves a record that holds a value and reads as deleted.
func TestDeleteBeforeWrite(t *testing.T) {
	d, err := Open(filepath.Join(t.TempDir(), "db"))
	require.NoError(t, err)
	defer func() { require.NoError(t, d.Close()) }()

	var hash [32]byte
	hash[0] = 3
	key := record.NewKey("Message", hash, "Main")

	batch := d.Begin(nil, true)
	require.NoError(t, batch.Delete(key))
	require.NoError(t, batch.Commit())

	put(t, d, key, "first")
	require.Equal(t, "first", get(t, d, key))

	// Nothing was refused: this is the silent case, which is why it
	// needs a test rather than a counter
	perm, _ := d.Stats()
	require.Zero(t, perm.PutConflict)
}
