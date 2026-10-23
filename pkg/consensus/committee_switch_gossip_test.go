// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package consensus_test

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"fmt"
	"sync"
	"testing"
	"time"

	pubsub "github.com/libp2p/go-libp2p-pubsub"
	"github.com/libp2p/go-libp2p/core/host"
	"github.com/libp2p/go-libp2p/core/peer"
	"github.com/libp2p/go-libp2p/core/protocol"
	bhost "github.com/libp2p/go-libp2p/p2p/host/blank"
	swarmt "github.com/libp2p/go-libp2p/p2p/net/swarm/testing"
	"github.com/prometheus/client_golang/prometheus/testutil"
	"github.com/stretchr/testify/require"
	"gitlab.com/accumulatenetwork/accumulate/pkg/consensus"
	"gitlab.com/accumulatenetwork/accumulate/pkg/consensus/gossip"
	"gitlab.com/accumulatenetwork/accumulate/pkg/consensus/metrics"
	"gitlab.com/accumulatenetwork/accumulate/pkg/consensus/types"
	"gitlab.com/accumulatenetwork/accumulate/pkg/consensus/worker"
)

// headerTrace records the headers a router delivered after its validator
// accepted them. pubsub does not report the router's own messages to a raw
// tracer, so a node's own headers are observed at its peers.
type headerTrace struct {
	topic string
	mu    sync.Mutex
	seen  []*types.Header
}

func (t *headerTrace) DeliverMessage(msg *pubsub.Message) {
	if msg.GetTopic() != t.topic {
		return
	}
	h, err := types.UnmarshalHeader(msg.Data)
	if err != nil {
		return
	}
	t.mu.Lock()
	t.seen = append(t.seen, h)
	t.mu.Unlock()
}

// delivered reports whether a header by author for epoch was delivered.
func (t *headerTrace) delivered(author ed25519.PublicKey, epoch uint64) bool {
	t.mu.Lock()
	defer t.mu.Unlock()
	for _, h := range t.seen {
		if h.Epoch == epoch && bytes.Equal(h.Author, author) {
			return true
		}
	}
	return false
}

func (*headerTrace) AddPeer(peer.ID, protocol.ID)          {}
func (*headerTrace) RemovePeer(peer.ID)                    {}
func (*headerTrace) Join(string)                           {}
func (*headerTrace) Leave(string)                          {}
func (*headerTrace) Graft(peer.ID, string)                 {}
func (*headerTrace) Prune(peer.ID, string)                 {}
func (*headerTrace) ValidateMessage(*pubsub.Message)       {}
func (*headerTrace) RejectMessage(*pubsub.Message, string) {}
func (*headerTrace) DuplicateMessage(*pubsub.Message)      {}
func (*headerTrace) ThrottlePeer(peer.ID)                  {}
func (*headerTrace) RecvRPC(*pubsub.RPC)                   {}
func (*headerTrace) SendRPC(*pubsub.RPC, peer.ID)          {}
func (*headerTrace) DropRPC(*pubsub.RPC, peer.ID)          {}
func (*headerTrace) UndeliverableMessage(*pubsub.Message)  {}

// TestCommitteeSwitchReachesGossip: a committee change given to
// Node.UpdateCommittee is what the node's router judges consensus messages
// by from then on (consensus.md, "Who may send consensus messages").
// Without it, every validator ignores every message of the new epoch after
// the first validator-set change.
//
// Two validators, A and B, start on an epoch-1 committee of A, B and R, and
// are switched to an epoch-2 committee of A, B and N. B is connected to A
// only, and an outside publisher S to A only, so what S publishes reaches
// B only if A's router relays it. An epoch-2 header signed by N must reach
// A and B; one signed by R must be refused by A and never reach B. And A's
// own first epoch-2 header must reach B: a router runs its validator on the
// node's own messages too, and a header the node's own router refuses is
// never sent, so gossip has to hold the new committee before the primary
// makes a header for it. The switch is made before the primaries start, so
// that header is the round-1 header; a switch while the primaries run
// stalls them on the headers of the round in flight until the committee is
// routed by round (#4593), and whether the primary or gossip learns the new
// committee first is then a matter of timing.
func TestCommitteeSwitchReachesGossip(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	const partition = "t4596switch"
	headersTopic := fmt.Sprintf(gossip.TopicHeaders, partition)

	newKey := func() (ed25519.PublicKey, ed25519.PrivateKey) {
		pub, priv, err := ed25519.GenerateKey(rand.Reader)
		require.NoError(t, err)
		return pub, priv
	}
	pubA, keyA := newKey()
	pubB, keyB := newKey()
	pubR, keyR := newKey()
	pubN, keyN := newKey()
	member := func(pub ed25519.PublicKey) types.ValidatorInfo {
		return types.ValidatorInfo{PublicKey: pub, Stake: 1}
	}
	epoch1 := types.NewCommittee([]types.ValidatorInfo{member(pubA), member(pubB), member(pubR)}, 1)
	epoch2 := types.NewCommittee([]types.ValidatorInfo{member(pubA), member(pubB), member(pubN)}, 2)

	// The routers before any connection (testutil.NewTestNetwork).
	newHost := func(trace *headerTrace) (host.Host, *pubsub.PubSub) {
		h := bhost.NewBlankHost(swarmt.GenSwarm(t))
		t.Cleanup(func() { _ = h.Close() })
		var opts []pubsub.Option
		if trace != nil {
			opts = append(opts, pubsub.WithRawTracer(trace))
		}
		ps, err := pubsub.NewGossipSub(ctx, h, opts...)
		require.NoError(t, err)
		return h, ps
	}
	traceA, traceB := &headerTrace{topic: headersTopic}, &headerTrace{topic: headersTopic}
	hA, psA := newHost(traceA)
	hB, psB := newHost(traceB)
	hS, psS := newHost(nil)
	for _, h := range []host.Host{hB, hS} {
		require.NoError(t, h.Connect(ctx, peer.AddrInfo{ID: hA.ID(), Addrs: hA.Addrs()}))
	}
	require.Empty(t, hB.Network().ConnsToPeer(hS.ID()), "B must not be connected to S")

	newNode := func(key ed25519.PrivateKey, h host.Host, ps *pubsub.PubSub) *consensus.Node {
		n, err := consensus.NewNode(consensus.NodeConfig{
			Partition:    partition,
			KeyPair:      key,
			NumWorkers:   1,
			WorkerConfig: worker.Config{BatchSize: 10, BatchTimeout: 50 * time.Millisecond},
		}, epoch1, h, ps)
		require.NoError(t, err)
		t.Cleanup(n.Stop)
		return n
	}
	nodeA, nodeB := newNode(keyA, hA, psA), newNode(keyB, hB, psB)
	require.Equal(t, uint64(1), nodeA.Committee().Epoch)

	// The switch: N joins, R leaves.
	for _, n := range []*consensus.Node{nodeA, nodeB} {
		n.UpdateCommittee(epoch2)
		require.NoError(t, n.InsertGenesisForAll([]ed25519.PrivateKey{keyA, keyB, keyN}))
	}
	refusedBefore := testutil.ToFloat64(metrics.GossipRefusedTotal.WithLabelValues(partition, "headers", "reject"))
	for _, n := range []*consensus.Node{nodeA, nodeB} {
		require.NoError(t, n.Start(ctx))
	}

	// S publishes on the raw topic and runs no validator.
	topic, err := psS.Join(headersTopic)
	require.NoError(t, err)
	signed := func(key ed25519.PrivateKey, pub ed25519.PublicKey) []byte {
		h := types.NewHeader(pub, 1, epoch2.Epoch, nil, nil)
		require.NoError(t, h.Sign(key))
		data, err := h.Marshal()
		require.NoError(t, err)
		return data
	}
	fromN, fromR := signed(keyN, pubN), signed(keyR, pubR)

	deadline := time.After(30 * time.Second)
	for !traceB.delivered(pubN, 2) || !traceB.delivered(pubA, 2) {
		require.NoError(t, topic.Publish(ctx, fromN))
		require.NoError(t, topic.Publish(ctx, fromR))
		select {
		case <-deadline:
			t.Fatalf("after the switch: N's header reached B %v, A's own epoch-2 header reached B %v",
				traceB.delivered(pubN, 2), traceB.delivered(pubA, 2))
		case <-time.After(300 * time.Millisecond):
		}
	}
	require.True(t, traceA.delivered(pubN, 2), "N's header was relayed by A without being delivered to it")

	require.False(t, traceA.delivered(pubR, 2), "R's header was delivered to A after R left the committee")
	require.False(t, traceB.delivered(pubR, 2), "R's header reached B after R left the committee")
	refused := testutil.ToFloat64(metrics.GossipRefusedTotal.WithLabelValues(partition, "headers", "reject"))
	require.Greater(t, refused, refusedBefore, "R's header was not rejected: it was ignored, or delivered")
}
