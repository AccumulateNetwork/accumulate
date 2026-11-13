// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package primary

import (
	"context"
	"slices"
	"testing"
	"time"

	pubsub "github.com/libp2p/go-libp2p-pubsub"
	"github.com/libp2p/go-libp2p/core/peer"
	bhost "github.com/libp2p/go-libp2p/p2p/host/blank"
	swarmt "github.com/libp2p/go-libp2p/p2p/net/swarm/testing"
	"github.com/stretchr/testify/require"
	"gitlab.com/accumulatenetwork/accumulate/pkg/consensus/gossip"
	"gitlab.com/accumulatenetwork/accumulate/pkg/consensus/types"
)

// A primary restarted at a round it never advanced from in this process
// (restored by SetRound and RestoreAuthored) still recovers from a stall: it
// re-shares its certificates and asks for the ones it is missing. A whole
// restart of a committee of two that stopped with each validator holding
// only its own certificate of the round otherwise waits for good, since
// nothing is pending and no one authors (netsim two-BVN restart: the
// Directory held at round 13).
func TestARestoredRoundThatNeverAdvancesRecoversFromTheStall(t *testing.T) {
	p, _ := restoredPrimary(t, 2)
	p.SetRound(1)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go func() { _ = p.Start(ctx) }()

	recovered := func() bool {
		p.roundSyncMu.Lock()
		defer p.roundSyncMu.Unlock()
		return !p.lastStallRecovery.IsZero()
	}
	time.Sleep(time.Second)
	require.False(t, recovered(), "a primary started a second ago was taken for stalled")
	require.Eventually(t, recovered, 10*time.Second, 50*time.Millisecond,
		"a restored round that never advanced was never recovered from the stall")

	p.roundMu.Lock()
	advanced := !p.lastRoundAdvance.IsZero()
	p.roundMu.Unlock()
	require.False(t, advanced, "the round advanced without a quorum")
}

// gossipPair is two gossip layers of the partition "restore" on connected
// in-process hosts, both judging messages by committee and started: the
// first is the primary's, the second a peer that only watches.
func gossipPair(t *testing.T, ctx context.Context, committee *types.Committee) (own, peerLayer *gossip.GossipLayer) {
	t.Helper()
	var layers [2]*gossip.GossipLayer
	var hosts [2]*bhost.BlankHost
	var routers [2]*pubsub.PubSub
	for i := range hosts {
		hosts[i] = bhost.NewBlankHost(swarmt.GenSwarm(t))
		h := hosts[i]
		t.Cleanup(func() { _ = h.Close() })
	}
	// The routers before the connection, as in the gossip package's tests
	for i := range hosts {
		ps, err := pubsub.NewGossipSub(ctx, hosts[i])
		require.NoError(t, err)
		routers[i] = ps
	}
	require.NoError(t, hosts[0].Connect(ctx, peer.AddrInfo{ID: hosts[1].ID(), Addrs: hosts[1].Addrs()}))
	for i := range layers {
		g, err := gossip.NewGossipLayer(hosts[i], routers[i], "restore")
		require.NoError(t, err)
		g.SetCommittee(committee)
		require.NoError(t, g.Start(ctx))
		t.Cleanup(func() { _ = g.Close() })
		layers[i] = g
	}
	return layers[0], layers[1]
}

// The stall recovery of a restored round, observed on the wire: a committee
// of two stopped with each validator holding only its own certificate of
// round 1. Restarted, the primary holds that certificate, nothing is pending
// and no round advances, so only recoverFromStall sends anything. After the
// hold time a peer sees it re-share its certificate of the round and ask for
// the certificates of the rounds around it, which the missing one of round 1
// is among.
func TestARestoredRoundReSharesItsCertificateAndRequestsTheRound(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	validators := []*testValidator{newTestValidator(t), newTestValidator(t)}
	committee := newTestCommittee(validators, 1)
	d := newTestDAG()
	genesis := createGenesisCertificates(t, validators, committee, d)
	var parents []types.CertificateDigest
	for _, c := range genesis {
		parents = append(parents, c.Digest())
	}
	v := validators[0]
	h := types.NewHeader(v.pub, 1, committee.Epoch, nil, parents)
	require.NoError(t, h.Sign(v.priv))
	sigs := make([][]byte, len(validators))
	authors := make([]uint16, len(validators))
	for i, voter := range validators {
		vote := types.NewVote(h.Digest(), h.Round, h.Epoch, voter.pub)
		require.NoError(t, vote.Sign(voter.priv))
		sigs[i], authors[i] = vote.Signature, uint16(i)
	}
	own := types.NewCertificate(h, sigs, authors)
	require.NoError(t, d.Insert(own))

	g, watcher := gossipPair(t, ctx, committee)
	p := New(Config{Partition: "restore", KeyPair: v.priv}, committee, g, d, nil)
	p.RestoreAuthored(h)
	p.pendingMu.Lock()
	_, collecting := p.ourHeaders[h.Digest()]
	p.pendingMu.Unlock()
	require.False(t, collecting, "the certified header went back into vote collection")
	require.Equal(t, types.Round(1), p.CurrentRound())

	go func() { _ = p.Start(ctx) }()

	var reShared, requested bool
	deadline := time.After(12 * time.Second)
	for !reShared || !requested {
		select {
		case c := <-watcher.SubscribeCertificates():
			if c.Digest() == own.Digest() {
				reShared = true
			}
		case req := <-watcher.SubscribeSyncRequests():
			if req.Requester.Equal(v.pub) && slices.Contains(req.Rounds, 1) && slices.Contains(req.Rounds, 2) {
				requested = true
			}
		case <-deadline:
			require.Truef(t, reShared, "the restored round's certificate was never re-shared")
			require.Truef(t, requested, "the restored round's certificates were never requested")
		}
	}

	p.roundMu.Lock()
	advanced := !p.lastRoundAdvance.IsZero()
	p.roundMu.Unlock()
	require.False(t, advanced, "the round advanced without a quorum")
}
