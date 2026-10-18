// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package api

import (
	"crypto/sha256"
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"
	"gitlab.com/accumulatenetwork/accumulate/internal/database"
	"gitlab.com/accumulatenetwork/accumulate/internal/database/indexing"
	"gitlab.com/accumulatenetwork/accumulate/pkg/database/keyvalue/memory"
	"gitlab.com/accumulatenetwork/accumulate/pkg/database/keyvalue/overlay"
	"gitlab.com/accumulatenetwork/accumulate/pkg/errors"
	"gitlab.com/accumulatenetwork/accumulate/pkg/types/encoding"
	"gitlab.com/accumulatenetwork/accumulate/pkg/url"
)

func entryHash(era string, i int) []byte {
	h := sha256.Sum256([]byte(fmt.Sprint(era, i)))
	return h[:]
}

func writeEntries(t *testing.T, batch *database.Batch, u *url.URL, era string, n int) {
	t.Helper()
	index := indexing.Data(batch, u)
	for i := 0; i < n; i++ {
		require.NoError(t, index.Put(entryHash(era, i), entryHash("txn "+era, i)))
	}
}

// #4273: a data index the reorg restarted is served as if it had not been:
// the pre-reorg entries first, then the live ones, numbered on.
func TestDataViewMapsARestartedIndex(t *testing.T) {
	u := url.MustParse("acc://restarted.acme/data")
	old := database.OpenInMemory(nil).Begin(true)
	defer old.Discard()
	writeEntries(t, old, u, "old", 409)
	// The live index is read as the API reads it: through the sidecar, which
	// holds the pre-reorg entries under the same numbers
	side := memory.New(nil)
	require.NoError(t, old.Commit())
	liveStore := memory.New(nil)
	live := database.New(overlay.Open(liveStore, side), nil).Begin(true)
	defer live.Discard()
	writeEntries(t, live, u, "new", 4)
	require.NoError(t, live.Commit())
	sideBatch := database.New(side, nil).Begin(true)
	writeEntries(t, sideBatch, u, "old", 409)
	require.NoError(t, sideBatch.Commit())
	live = database.New(overlay.Open(liveStore, side), nil).Begin(false)
	defer live.Discard()

	s := new(Querier)
	v, err := s.dataView(live, &indexing.Era{Batch: old}, u)
	require.NoError(t, err)
	require.EqualValues(t, 409, v.pre)

	n, err := v.Count()
	require.NoError(t, err)
	require.EqualValues(t, 413, n)
	for i, want := range map[uint64][]byte{0: entryHash("old", 0), 408: entryHash("old", 408), 409: entryHash("new", 0), 412: entryHash("new", 3)} {
		got, err := v.Entry(i)
		require.NoError(t, err)
		require.Equal(t, want, got, "entry %d", i)
	}
	_, err = v.Entry(413)
	require.ErrorIs(t, err, errors.NotFound, "the sidecar's entry 4 leaked through the live index")

	// Remembered: the second view does not read the era's count again
	_, ok := s.dataViews.Load(u.String())
	require.True(t, ok)
}

// Where the era's count is the live one (no overwritten store holds the
// pre-reorg count), the run of entries says how many there were.
func TestDataViewFindsThePreReorgCountFromTheEntries(t *testing.T) {
	u := url.MustParse("acc://bitcoin.acme/price")
	store := memory.New(nil)
	db := database.New(store, nil)
	old := db.Begin(true)
	writeEntries(t, old, u, "old", 1000)
	countKey := old.Account(u).Data().Entry().Key()
	require.NoError(t, old.Commit())
	// The count record is the live database's, restarted: 7
	cs := store.Begin(nil, true)
	require.NoError(t, cs.Put(countKey, encoding.MarshalUint(7)))
	require.NoError(t, cs.Commit())
	era := db.Begin(false)
	defer era.Discard()
	n, err := indexing.Data(era, u).Count()
	require.NoError(t, err)
	require.EqualValues(t, 7, n)

	live := database.OpenInMemory(nil).Begin(true)
	defer live.Discard()
	writeEntries(t, live, u, "new", 7)

	s := new(Querier)
	v, err := s.dataView(live, &indexing.Era{Batch: era}, u)
	require.NoError(t, err)
	require.EqualValues(t, 1000, v.pre)
	n, err = v.Count()
	require.NoError(t, err)
	require.EqualValues(t, 1007, n)
}

// An index the restore carried whole is served as it is: the era holds no
// entry at the live count.
func TestDataViewLeavesAWholeIndexAlone(t *testing.T) {
	u := url.MustParse("acc://whole.acme/data")
	old := database.OpenInMemory(nil).Begin(true)
	defer old.Discard()
	writeEntries(t, old, u, "old", 3)
	live := database.OpenInMemory(nil).Begin(true)
	defer live.Discard()
	writeEntries(t, live, u, "old", 3)
	writeEntries(t, live, u, "new", 2) // written since, numbered on

	s := new(Querier)
	v, err := s.dataView(live, &indexing.Era{Batch: old}, u)
	require.NoError(t, err)
	require.EqualValues(t, 0, v.pre)
	n, err := v.Count()
	require.NoError(t, err)
	require.EqualValues(t, 5, n)

	// And without an era, nothing changes
	v, err = s.dataView(live, nil, url.MustParse("acc://other.acme/data"))
	require.NoError(t, err)
	require.Nil(t, v.era)
}
