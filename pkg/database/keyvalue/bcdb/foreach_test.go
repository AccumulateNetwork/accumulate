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

// Iteration is a read like any other: it sees the database as of the
// batch's version, not as of now (#4175).
//
// Everything here stays STAGED (a holder batch pins version 0), because
// the store's own ForEach holds its mutex across the callback and a
// callback that reads the store would deadlock -- BlockchainDB#31.
// What this proves is the adapter's half: the version is honoured, and
// the adapter's lock is not held across the callback.
func TestForEach_SeesTheBatchVersion(t *testing.T) {
	d, err := Open(filepath.Join(t.TempDir(), "db"))
	require.NoError(t, err)
	defer d.Close()

	holder := d.Begin(nil, false) // keeps every later commit staged
	defer holder.Discard()

	key := record.NewKey("Account", "alice", "Main")
	put(t, d, key, "old")

	old := d.Begin(nil, false)
	defer old.Discard()
	put(t, d, key, "new") // newer than `old`

	var saw string
	require.NoError(t, old.ForEach(func(k *record.Key, v []byte) error {
		if k.Hash() == key.Hash() {
			saw = string(v)
		}
		return nil
	}))
	require.Equal(t, "old", saw, "a batch must not iterate over a commit newer than itself")

	// The callback may read the database while iterating: the adapter's
	// lock is not held across it.
	require.NoError(t, old.ForEach(func(k *record.Key, v []byte) error {
		got, err := old.Get(key)
		require.Equal(t, "old", string(got))
		return err
	}))
}

// The tally remembers a bounded number of keys and says so.
func TestTally_IsBounded(t *testing.T) {
	d, err := Open(filepath.Join(t.TempDir(), "db"))
	require.NoError(t, err)
	defer d.Close()
	d.TallyKeys = 2

	for i := 0; i < 5; i++ {
		put(t, d, record.NewKey("Message", [32]byte{byte(i)}, "Main"), "v")
	}
	require.Len(t, d.last, 2, "digests remembered up to the cap and no further")
	require.Equal(t, uint64(5), d.Shapes()["Message.(hash).Main"].New, "every write is still counted")
}
