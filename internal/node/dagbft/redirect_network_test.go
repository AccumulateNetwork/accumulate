// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package dagbft

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/sha256"
	"fmt"
	"sync/atomic"
	"testing"
	"time"

	"github.com/libp2p/go-libp2p"
	dht "github.com/libp2p/go-libp2p-kad-dht"
	pubsub "github.com/libp2p/go-libp2p-pubsub"
	"github.com/libp2p/go-libp2p/core/host"
	"github.com/libp2p/go-libp2p/core/peer"
	"github.com/multiformats/go-multiaddr"
	"github.com/prometheus/client_golang/prometheus/testutil"
	"github.com/stretchr/testify/require"
	"gitlab.com/accumulatenetwork/accumulate/internal/core/events"
	"gitlab.com/accumulatenetwork/accumulate/internal/database"
	"gitlab.com/accumulatenetwork/accumulate/pkg/api/v3"
	"gitlab.com/accumulatenetwork/accumulate/pkg/api/v3/message"
	"gitlab.com/accumulatenetwork/accumulate/pkg/api/v3/p2p"
	"gitlab.com/accumulatenetwork/accumulate/pkg/consensus"
	"gitlab.com/accumulatenetwork/accumulate/pkg/consensus/adapter"
	"gitlab.com/accumulatenetwork/accumulate/pkg/consensus/metrics"
	"gitlab.com/accumulatenetwork/accumulate/pkg/consensus/worker"
	"gitlab.com/accumulatenetwork/accumulate/pkg/errors"
	"gitlab.com/accumulatenetwork/accumulate/pkg/types/messaging"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

// notLoadedCount is a node's sections not loaded, as the join's state
// reports them.
type notLoadedCount struct{ n atomic.Int64 }

func (c *notLoadedCount) SectionsNotLoaded() int { return int(c.n.Load()) }

// gatedAdapter validates as a node with sections not loaded does: an
// envelope that needs one -- marked by the test -- is refused NotReady, in
// the shape the executor bridge returns the store's refusal ("validate: ...
// is in section N of the state tree, which this node has not loaded yet").
type gatedAdapter struct {
	*executingAdapter
	sections *notLoadedCount
	needs    [][]byte
}

func (a *gatedAdapter) ValidateTransaction(tx []byte) error {
	if a.sections.SectionsNotLoaded() > 0 {
		for _, m := range a.needs {
			if bytes.Contains(tx, m) {
				return fmt.Errorf("validate: %w", errors.UnknownError.WithFormat("load signer: %w",
					database.NotLoadedError(1017, "acc://dave/book/1 is in section 1017 of the state tree, which this node has not loaded yet")))
			}
		}
	}
	return a.executingAdapter.ValidateTransaction(tx)
}

// TestASubmissionNeedingASectionNotLoadedIsRedirected — #4485, Paul
// 2026-10-02 (executor spec, "Sync", Redirection; DIFFERENCES E25). Four real
// pkg/consensus validators, each with a real p2p node and the production
// submit and consensus handlers, the production Relay over the production
// client and dialer. What a node's executor would refuse for a section it has
// not loaded is stood in by the adapter, in the bridge's error shape; the
// rest is production.
//
//   - A submission to a node with the section not loaded is validated there,
//     refused NotReady, redirected to a node holding every section, and lands
//     in a block on the network; the caller gets that node's answer.
//   - A node holding every section validates locally and never redirects.
//   - A submission no node can validate -- every node has sections not
//     loaded -- is refused, not passed round.
func TestASubmissionNeedingASectionNotLoadedIsRedirected(t *testing.T) {
	if testing.Short() {
		t.Skip("four consensus nodes and their libp2p hosts")
	}
	const (
		nVals   = 4
		part    = "BVN5redirect"
		netName = "RedirectNet"
	)
	ctx0 := context.Background()

	keys := make([]ed25519.PrivateKey, nVals)
	pubs := make([]ed25519.PublicKey, nVals)
	for i := range keys {
		pub, priv, err := ed25519.GenerateKey(rand.Reader)
		require.NoError(t, err)
		keys[i], pubs[i] = priv, pub
	}
	globals := globalsWith(t, map[string][]ed25519.PublicKey{part: pubs})

	chosts := make([]host.Host, nVals)
	gossips := make([]*pubsub.PubSub, nVals)
	for i := range chosts {
		h, err := libp2p.New(libp2p.ListenAddrStrings("/ip4/127.0.0.1/tcp/0"))
		require.NoError(t, err)
		chosts[i] = h
		t.Cleanup(func() { _ = h.Close() })
		gossips[i], err = pubsub.NewGossipSub(ctx0, h)
		require.NoError(t, err)
	}
	for i := range chosts {
		for j := i + 1; j < len(chosts); j++ {
			require.NoError(t, chosts[i].Connect(ctx0, peer.AddrInfo{ID: chosts[j].ID(), Addrs: chosts[j].Addrs()}))
		}
	}
	initial := make([]adapter.ValidatorInfo, nVals)
	for i := range initial {
		initial[i] = adapter.ValidatorInfo{PublicKey: [32]byte(pubs[i]), Stake: 100, Active: true}
	}

	type node struct {
		svc      *Service
		exec     *gatedAdapter
		api      *p2p.Node
		sections *notLoadedCount
	}
	nodes := make([]*node, nVals)
	var apiAddrs []multiaddr.Multiaddr
	for i := range nodes {
		sections := new(notLoadedCount)
		exec := &gatedAdapter{executingAdapter: newExecutingAdapter(), sections: sections,
			needs: [][]byte{[]byte("NEEDS-"), []byte("NONE-")}}
		svc, err := NewService(ServiceConfig{
			Partition: &protocol.PartitionInfo{ID: part, Type: protocol.PartitionTypeBlockValidator},
			NodeConfig: consensus.NodeConfig{
				Partition:  part,
				KeyPair:    keys[i],
				NumWorkers: 4,
				WorkerConfig: worker.Config{
					Partition:    part,
					BatchSize:    20,
					BatchTimeout: 200 * time.Millisecond,
				},
				MinRoundInterval:    50 * time.Millisecond,
				BatchCollectTimeout: 10 * time.Second,
			},
			Adapter:           exec,
			EventBus:          events.NewBus(nil),
			Host:              chosts[i],
			PubSub:            gossips[i],
			InitialValidators: initial,
			Database:          database.OpenInMemory(nil),
		})
		require.NoError(t, err)

		apiNode, err := p2p.New(p2p.Options{
			Network:        netName,
			Listen:         []multiaddr.Multiaddr{multiaddr.StringCast("/ip4/127.0.0.1/tcp/0")},
			BootstrapPeers: apiAddrs,
			DiscoveryMode:  dht.ModeServer,
		})
		require.NoError(t, err)
		t.Cleanup(func() { _ = apiNode.Close() })
		apiAddrs = append(apiAddrs, apiNode.Addresses()...)

		// As cmd/accumulated/run builds them, with the join's sections.
		m := NewMembership(part, pubs[i])
		m.SetGlobals(globals)
		relay := NewRelay(RelayParams{
			Partition:  part,
			Membership: m,
			Peers:      NodeRelayPeers{Node: apiNode},
			RPC: ClientRelayRPC{
				Partition: part,
				Client: &message.Client{Transport: &message.RoutedTransport{
					Network: netName,
					Dialer:  apiNode.DialNetwork(),
				}},
			},
		})
		sub := NewSubmitterService(SubmitterServiceParams{
			Service: svc, Membership: m, Relay: relay, Sections: sections,
		})
		cons := NewConsensusAPIService(ConsensusAPIServiceParams{
			Service:          svc,
			PartitionID:      part,
			PartitionType:    protocol.PartitionTypeBlockValidator,
			ValidatorKeyHash: sha256.Sum256(pubs[i]),
			Sections:         sections,
			ValidatorKey:     keys[i],
			PeerID:           apiNode.ID(),
		})
		registerService(t, apiNode, api.ServiceTypeSubmit.AddressFor(part), message.Submitter{Submitter: sub})
		registerService(t, apiNode, api.ServiceTypeConsensus.AddressFor(part), message.ConsensusService{ConsensusService: cons})
		nodes[i] = &node{svc: svc, exec: exec, api: apiNode, sections: sections}
	}

	ctx, cancel := context.WithCancel(ctx0)
	defer cancel()
	for _, nd := range nodes {
		require.NoError(t, nd.svc.Start(ctx))
		nd := nd
		t.Cleanup(func() { _ = nd.svc.Stop() })
	}
	require.Eventually(t, func() bool {
		return len(nodes[0].api.Providers(ctx, api.ServiceTypeSubmit.AddressFor(part), 10)) >= nVals-1
	}, 30*time.Second, 200*time.Millisecond, "the API mesh never formed")
	time.Sleep(2 * time.Second) // the consensus mesh

	peerNode, err := p2p.New(p2p.Options{
		Network:        netName,
		Listen:         []multiaddr.Multiaddr{multiaddr.StringCast("/ip4/127.0.0.1/tcp/0")},
		BootstrapPeers: apiAddrs,
		DiscoveryMode:  dht.ModeServer,
	})
	require.NoError(t, err)
	defer func() { _ = peerNode.Close() }()
	client := &message.Client{Transport: &message.RoutedTransport{Network: netName, Dialer: peerNode.DialNetwork()}}
	submitTo := func(i int, mark string) ([]*api.Submission, error) {
		env := new(messaging.Envelope)
		env.TxHash = []byte(mark)
		verify := false
		return client.ForPeer(nodes[i].api.ID()).
			ForAddress(api.ServiceTypeSubmit.AddressFor(part).Multiaddr()).
			Submit(ctx, env, api.SubmitOptions{Verify: &verify})
	}
	relayed := func(outcome string) float64 {
		return testutil.ToFloat64(metrics.RelayedTotal.WithLabelValues(part, outcome))
	}
	executed := func(mark string) bool {
		for _, nd := range nodes {
			if nd.exec.count(mark) > 0 {
				return true
			}
		}
		return false
	}

	// Node 0 has a section not loaded; the others hold every section.
	nodes[0].sections.n.Store(1)

	// (a) Node 0 cannot validate it, and redirects it.
	taken0 := relayed(metrics.RelayTaken)
	res, err := submitTo(0, "NEEDS-SECTION-1")
	require.NoError(t, err, "a submission node 0 could not validate was refused, not redirected")
	require.Len(t, res, 1)
	require.True(t, res[0].Success, "the redirected submission's answer: %v", res[0].Message)
	require.Equal(t, taken0+1, relayed(metrics.RelayTaken), "the submission was not taken by another node")
	require.Eventually(t, func() bool { return executed("NEEDS-SECTION-1") }, 20*time.Second, 100*time.Millisecond,
		"the redirected submission never landed in a block")

	// (b) A node holding every section validates locally, and redirects
	// nothing -- the same envelope, at node 1.
	all := func() float64 {
		n := float64(0)
		for _, o := range []string{metrics.RelayTaken, metrics.RelayRefused, metrics.RelayNotReady, metrics.RelayUnreachable} {
			n += relayed(o)
		}
		return n
	}
	before := all()
	res, err = submitTo(1, "NEEDS-SECTION-2")
	require.NoError(t, err)
	require.True(t, res[0].Success)
	require.Equal(t, before, all(), "a node holding every section redirected a submission")
	require.Eventually(t, func() bool { return executed("NEEDS-SECTION-2") }, 20*time.Second, 100*time.Millisecond)

	// (c) Every node has a section not loaded: nobody can validate it. It is
	// refused, once, and not passed round.
	for _, nd := range nodes {
		nd.sections.n.Store(1)
	}
	before = all()
	notReady0 := relayed(metrics.RelayNotReady) + relayed(metrics.RelayUnreachable)
	_, err = submitTo(0, "NONE-CAN-1")
	require.Error(t, err, "a submission no node can validate was accepted")
	require.Equal(t, before+1, all(), "the submission was relayed more than once, or not at all")
	require.Equal(t, notReady0+1, relayed(metrics.RelayNotReady)+relayed(metrics.RelayUnreachable),
		"the submission no node can validate did not end not-ready")
	time.Sleep(time.Second)
	require.False(t, executed("NONE-CAN-1"), "a submission no node could validate was executed")
}
