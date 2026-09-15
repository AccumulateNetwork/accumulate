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
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

// A reader that wants the latest state -- CheckTx, which validates against
// whatever has been committed -- pins no version. A commit made while it
// is open takes no pre-images (there is nobody to keep them for), and the
// reader sees the commit. Before #4237 every CheckTx registered a view, so
// a reader was almost always open while a block committed and every commit
// paid a GetDyna per dynamic entry.
func TestUnisolatedReaderTakesNoPreImages(t *testing.T) {
	d, err := Open(filepath.Join(t.TempDir(), "db"))
	require.NoError(t, err)
	defer func() { require.NoError(t, d.Close()) }()

	key := record.NewKey("Account", "alice", "Main")
	put(t, d, key, "old")

	u := d.BeginUnisolated(nil, false)
	defer u.Discard()
	v, err := u.Get(key)
	require.NoError(t, err)
	require.Equal(t, "old", string(v))

	put(t, d, key, "new")
	require.Zero(t, d.PreImageReads(), "no reader pinned a version, so nothing was read for one")
	require.Empty(t, d.undoVersions, "and no overlay is held")

	v, err = u.Get(key)
	require.NoError(t, err)
	require.Equal(t, "new", string(v), "an unisolated reader reads the latest state")

	// The contrast: an ordinary reader pins its version and is kept there
	r := d.Begin(nil, false)
	defer r.Discard()
	put(t, d, key, "newer")
	require.Equal(t, uint64(1), d.PreImageReads(), "one dynamic key rewritten under a pinned reader")
	v, err = r.Get(key)
	require.NoError(t, err)
	require.Equal(t, "new", string(v))
	v, err = u.Get(key)
	require.NoError(t, err)
	require.Equal(t, "newer", string(v))
}

// The unisolated batch is a change set like any other: it writes, it
// commits, its children see its writes, and ForEach sees the store.
func TestUnisolatedBatchWritesAndIterates(t *testing.T) {
	d, err := Open(filepath.Join(t.TempDir(), "db"))
	require.NoError(t, err)
	defer func() { require.NoError(t, d.Close()) }()

	put(t, d, record.NewKey("Account", "a", "Main"), "a")

	u := d.BeginUnisolated(nil, true)
	require.NoError(t, u.Put(record.NewKey("Account", "b", "Main"), []byte("b")))
	child := u.Begin(nil, false)
	v, err := child.Get(record.NewKey("Account", "b", "Main"))
	require.NoError(t, err)
	require.Equal(t, "b", string(v))
	child.Discard()
	require.NoError(t, u.Commit())

	require.Equal(t, "b", get(t, d, record.NewKey("Account", "b", "Main")))

	u = d.BeginUnisolated(nil, false)
	defer u.Discard()
	n := 0
	require.NoError(t, u.ForEach(func(*record.Key, []byte) error { n++; return nil }))
	require.Equal(t, 2, n)
}

// With a reader pinned, a commit reads a pre-image for the dynamic keys the
// adapter cannot account for itself and nothing else: a permanent shape has
// no pre-image by definition, and a record the adapter caches as immutable
// (Account.Url) is answered from the cache -- present means "this is its
// value", absent means "it is new". Whether a dynamic key of any other
// shape is new is the store's to say, and it says so by walking history
// (DIFFERENCES D10).
func TestPreImagesAreReadOnlyForUnaccountedDynamicKeys(t *testing.T) {
	d, err := Open(filepath.Join(t.TempDir(), "db"))
	require.NoError(t, err)
	defer func() { require.NoError(t, d.Close()) }()

	existing := []*record.Key{
		record.NewKey("Account", "alice", "Main"),
		record.NewKey("Account", "bob", "Main"),
	}
	cachedURL := record.NewKey("Account", protocol.AccountUrl("alice"), "Url")
	batch := d.Begin(nil, true)
	for _, k := range existing {
		require.NoError(t, batch.Put(k, []byte("old")))
	}
	require.NoError(t, batch.Put(cachedURL, []byte("acc://alice")))
	require.NoError(t, batch.Commit())

	reader := d.Begin(nil, false)
	defer reader.Discard()

	before := d.PreImageReads()
	batch = d.Begin(nil, true)
	for _, k := range existing { // 2 pre-existing dynamic keys
		require.NoError(t, batch.Put(k, []byte("new")))
	}
	for i := 0; i < 3; i++ { // 3 new dynamic keys
		require.NoError(t, batch.Put(record.NewKey("Account", fmt.Sprint("new", i), "Pending"), []byte("p")))
	}
	for i := 0; i < 4; i++ { // 4 permanent keys
		require.NoError(t, batch.Put(record.NewKey("Message", [32]byte{byte(i + 1)}, "Main"), []byte("m")))
	}
	require.NoError(t, batch.Put(cachedURL, []byte("acc://alice")))                                                     // 1 cached, present
	require.NoError(t, batch.Put(record.NewKey("Account", protocol.AccountUrl("carol"), "Url"), []byte("acc://carol"))) // 1 cached shape, new
	require.NoError(t, batch.Commit())

	require.Equal(t, uint64(5), d.PreImageReads()-before,
		"2 pre-existing + 3 new dynamic keys of unaccounted shapes; none for the 4 permanent or the 2 URL keys")

	// The reader is still at its version for all of them
	for _, k := range existing {
		v, err := reader.Get(k)
		require.NoError(t, err)
		require.Equal(t, "old", string(v))
	}
	v, err := reader.Get(cachedURL)
	require.NoError(t, err)
	require.Equal(t, "acc://alice", string(v), "a cached record's pre-image is its value")
	_, err = reader.Get(record.NewKey("Account", protocol.AccountUrl("carol"), "Url"))
	var nf *database.NotFoundError
	require.ErrorAs(t, err, &nf, "a record that was not cached was new")
	_, err = reader.Get(record.NewKey("Account", "new0", "Pending"))
	require.ErrorAs(t, err, &nf)
}
