// Copyright 2025 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package light

import (
	"context"
	"fmt"
	"os/user"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
	"gitlab.com/accumulatenetwork/accumulate/internal/database/record"
	"gitlab.com/accumulatenetwork/accumulate/pkg/accumulate"
	"gitlab.com/accumulatenetwork/accumulate/pkg/api/v3"
	"gitlab.com/accumulatenetwork/accumulate/pkg/api/v3/jsonrpc"
	"gitlab.com/accumulatenetwork/accumulate/pkg/database/keyvalue/bolt"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

func TestIndex(t *testing.T) {
	entries := []*protocol.IndexEntry{
		{Source: 4},
		{Source: 8},
	}

	cases := []struct {
		Source uint64
		Result int
	}{
		{0, 0},
		{2, 0},
		{4, 0},
		{6, 1},
		{8, 1},
	}

	for i, c := range cases {
		t.Run(fmt.Sprintf("%d", i), func(t *testing.T) {
			i, _, ok := FindEntry(entries, ByIndexSource(c.Source))
			require.True(t, ok)
			require.Equal(t, c.Result, i)
		})
	}
}

func TestDeleteOldIndices(t *testing.T) {
	t.Skip("Manual")

	ns, err := jsonrpc.NewClient(accumulate.ResolveWellKnownEndpoint("kermit", "v3")).NetworkStatus(context.Background(), api.NetworkStatusOptions{})
	require.NoError(t, err)

	cu, err := user.Current()
	require.NoError(t, err)
	store, err := bolt.Open(filepath.Join(cu.HomeDir, ".accumulate", "cache", "kermit.db"), bolt.WithPlainKeys)
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, store.Close()) })

	db := OpenDB(store, nil, false)
	defer db.Discard()
	var keys []*record.Key
	for _, a := range ns.Network.Partitions {
		for _, b := range ns.Network.Partitions {
			index := db.Index().Partition(protocol.PartitionUrl(a.ID)).Anchors().Received(protocol.PartitionUrl(b.ID))
			keys = append(keys, index.Key())
		}
	}
	db.Discard()

	batch := store.Begin(nil, true)
	defer batch.Discard()
	for _, k := range keys {
		require.NoError(t, batch.Delete(k))
	}
	require.NoError(t, batch.Commit())
}
