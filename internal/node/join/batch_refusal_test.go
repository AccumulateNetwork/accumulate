// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package join

import (
	"context"
	"math/big"
	"testing"

	"github.com/stretchr/testify/require"
	apiimpl "gitlab.com/accumulatenetwork/accumulate/internal/api/v3"
	"gitlab.com/accumulatenetwork/accumulate/internal/core/bootstrap/pull"
	"gitlab.com/accumulatenetwork/accumulate/internal/database"
	"gitlab.com/accumulatenetwork/accumulate/pkg/api/v3"
	"gitlab.com/accumulatenetwork/accumulate/pkg/url"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

// bentStates is a peer whose account states carry a body other than the one
// its leaf was hashed from: a reply torn, or wrong.
type bentStates struct {
	pull.Source
	name  string
	asked *int
}

func (b bentStates) String() string { return b.name }

func (b bentStates) QueryAccountStates(ctx context.Context, scope *url.URL, q *api.AccountStatesQuery) (*api.AccountStatesRecord, error) {
	*b.asked++
	r, err := b.Source.QueryAccountStates(ctx, scope, q)
	if err != nil {
		return nil, err
	}
	for _, st := range r.States {
		if a, ok := st.Main.(*protocol.TokenAccount); ok {
			a = a.Copy()
			a.Balance = *big.NewInt(12)
			st.Main = a
		}
	}
	return r, nil
}

type named struct {
	pull.Source
	name string
}

func (n named) String() string { return n.name }

// TestAnAccountWhosePartsDoNotHashToItsLeafIsRefused — #4453, executor spec,
// "Sync", §3: "The node hashes the state it was served, as hashState does, and
// compares it with the leaf hash served beside it; a mismatch is that peer's
// reply refused, and the account is asked of the next peer." Peer a serves
// alice's leaf beside a body it was not hashed from; peer b serves her whole.
// She is refused from a and taken from b; with a alone she is owed, and
// nothing a served is written.
func TestAnAccountWhosePartsDoNotHashToItsLeafIsRefused(t *testing.T) {
	here := protocol.PartitionUrl("BVN0")
	alice := protocol.AccountUrl("alice", "tokens")
	peerDB := peerStore(t)
	putLedger(t, peerDB, here, 10)
	putTokens(t, peerDB, alice, 5)
	prod := api.Querier2{Querier: apiimpl.NewQuerier(apiimpl.QuerierParams{Database: peerDB, Partition: "BVN0"})}
	var asked int
	bent := bentStates{Source: prod, name: "peer a", asked: &asked}
	honest := named{Source: prod, name: "peer b"}

	joining := func(srcs ...pull.Source) (*PulledState, *database.Database) {
		db := database.OpenInMemory(nil)
		db.SetObserver(database.NewDatabaseObserver())
		t.Cleanup(func() { _ = db.Close() })
		values, _ := genesisValues(t, 4)
		putNetwork(t, db, here, values)
		s, err := NewState(StateOptions{Partition: here, Database: db, Sources: &fixedSources{partition: here, srcs: srcs}, ExecutedBlock: 1})
		require.NoError(t, err)
		return s, db
	}
	has := func(db *database.Database) bool {
		b := db.Begin(false)
		defer b.Discard()
		_, err := b.Account(alice).Main().Get()
		return err == nil
	}

	// a alone: refused, owed, nothing written.
	s, db := joining(bent)
	require.NoError(t, s.Pull(context.Background()))
	require.True(t, s.sync.walked, "precondition: the walk covered the tree")
	require.False(t, has(db), "alice was written from a reply whose parts do not hash to its leaf")
	require.Contains(t, s.sync.retry, accountKey(alice), "alice was not owed after the only peer's reply was refused")

	// a, then b: taken from b, as b serves her.
	asked = 0
	s, db = joining(bent, honest)
	require.NoError(t, s.Pull(context.Background()))
	require.NotZero(t, asked, "precondition: peer a was asked for alice")
	require.True(t, has(db), "alice was not asked of the next peer")
	require.Equal(t, int64(5), balanceIn(t, db, alice), "alice was taken from the peer whose reply does not hash to its leaf")
	require.Equal(t, leafIn(t, peerDB, alice).ValueHash, leafIn(t, db, alice).ValueHash)
	require.NotContains(t, s.sync.retry, accountKey(alice))
	require.True(t, s.sync.servedBy["peer b"], "precondition: peer b served alice")
}
