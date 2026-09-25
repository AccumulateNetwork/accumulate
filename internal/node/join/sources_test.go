// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package join

import (
	"context"
	"testing"

	"github.com/libp2p/go-libp2p/core/peer"
	"github.com/stretchr/testify/require"
	"gitlab.com/accumulatenetwork/accumulate/internal/core/bootstrap/pull"
	"gitlab.com/accumulatenetwork/accumulate/internal/database"
	"gitlab.com/accumulatenetwork/accumulate/pkg/api/v3"
	"gitlab.com/accumulatenetwork/accumulate/pkg/errors"
	"gitlab.com/accumulatenetwork/accumulate/pkg/url"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

// TestSelectPeers_NeverThisNode is the guard on #4303.
//
// The join used to be handed the node's own routed client. That client answers
// locally for any service the node provides -- p2p.DialNetwork installs a
// self-discoverer unconditionally and every partition a node serves registers
// query:<id> -- so the pull read the un-executed store it exists to fill and
// was refused by it 18,313 times without ever reaching the network. Every peer
// list the pull is built from drops this node, so the routed client cannot
// come back by accident.
func TestSelectPeers_NeverThisNode(t *testing.T) {
	self := peer.ID("self")
	other := peer.ID("other")

	got := selectPeers([]*api.FindServiceResult{
		{PeerID: self},
		{PeerID: other},
		{PeerID: ""},
		nil,
	}, self)

	require.Equal(t, []peer.ID{other}, got)

	// And a node alone with itself has nobody to pull from. That is an error,
	// not an empty list that reads as "nothing to do": a join that concluded
	// it had caught up because nobody answered would execute from a state no
	// peer holds (#4296).
	require.Empty(t, selectPeers([]*api.FindServiceResult{{PeerID: self}}, self))
}

// TestQueryPeers_RefusesWithNoPeerButItself — the error a node alone gets, in
// the words an operator reads.
func TestQueryPeers_RefusesWithNoPeerButItself(t *testing.T) {
	q := &QueryPeers{}
	_, err := q.ForPartition(context.Background(), protocol.PartitionUrl("BVN0"))
	require.Error(t, err, "a QueryPeers with no client cannot produce a source")
}

// TestNewState_NeedsPeersNotAQuerier — the shape of the fix, pinned.
//
// StateOptions used to take a Query, and the daemon gave it the node's own
// routed client. It takes Sources now, and a Sources is a thing that names
// peers. A caller that has only a querier cannot satisfy it, which is the
// point: there is no querier a joining node may pull from, including its own.
func TestNewState_NeedsPeersNotAQuerier(t *testing.T) {
	db := database.OpenInMemory(nil)
	db.SetObserver(database.NewDatabaseObserver())
	values, _ := genesisValues(t, 4)
	putNetwork(t, db, protocol.PartitionUrl("BVN0"), values)

	_, err := NewState(StateOptions{
		Partition: protocol.PartitionUrl("BVN0"),
		Database:  db,
	})
	require.Error(t, err, "a join's state was built with no peers to pull from")
	require.True(t, errors.Is(err, errors.BadRequest), "got %v", err)

	_, err = NewState(StateOptions{
		Partition: protocol.PartitionUrl("BVN0"),
		Database:  db,
		Sources:   noSources{},
	})
	require.NoError(t, err)
}

type noSources struct{ noValidators }

func (noSources) For(context.Context, *url.URL) ([]pull.Source, *url.URL, error) {
	return nil, nil, errors.NotReady.With("no peers")
}

// A querier that answers nothing, which is what a Sources with no peers has
// to hand over. It is not nil: the anchor source refuses a nil querier,
// because a join whose roots come from nowhere verifies nothing (#4301).
func (noSources) Querier(*url.URL) api.Querier { return noQuerier{} }

type noQuerier struct{}

func (noQuerier) Query(context.Context, *url.URL, api.Query) (api.Record, error) {
	return nil, errors.NotReady.With("no peers")
}
