// Copyright 2023 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package dial

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/libp2p/go-libp2p/core/peer"
	"github.com/stretchr/testify/require"
	"gitlab.com/accumulatenetwork/accumulate/pkg/api/v3"
	"gitlab.com/accumulatenetwork/accumulate/pkg/api/v3/p2p/peerdb"
)

func TestTrackerPersistentDiscovery(t *testing.T) {
	p1 := newPeer(t, 1)
	p2 := newPeer(t, 2)
	p3 := newPeer(t, 3)

	db := peerdb.New()
	db.Peer(p1).Network(t.Name()).Service(api.ServiceTypeNode.Address()).Last.DidSucceed()
	db.Peer(p2).Network(t.Name()).Service(api.ServiceTypeNode.Address()).Last.DidSucceed()
	db.Peer(p3).Network(t.Name()).Service(api.ServiceTypeNode.Address()).Last.DidSucceed()

	file := filepath.Join(t.TempDir(), "peerdb.json")
	require.NoError(t, db.StoreFile(file))

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	tracker := &PersistentTracker{
		context: ctx,
		cancel:  cancel,
		db:      db,
		file:    file,
		network: t.Name(),
	}

	res, err := tracker.Discover(ctx, &DiscoveryRequest{})
	require.NoError(t, err)
	require.IsType(t, make(DiscoveredPeers), res)

	var peers []peer.ID
	for p := range res.(DiscoveredPeers) {
		peers = append(peers, p.ID)
	}
	require.ElementsMatch(t, []peer.ID{p1, p2, p3}, peers)
}
