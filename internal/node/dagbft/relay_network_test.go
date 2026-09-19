// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package dagbft

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/sha256"
	"fmt"
	"net/http/httptest"
	"strings"
	"sync"
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
	"gitlab.com/accumulatenetwork/accumulate/internal/core/bootstrap/nodestate"
	"gitlab.com/accumulatenetwork/accumulate/internal/core/events"
	nodehttp "gitlab.com/accumulatenetwork/accumulate/internal/node/http"
	"gitlab.com/accumulatenetwork/accumulate/pkg/api/v3"
	"gitlab.com/accumulatenetwork/accumulate/pkg/api/v3/jsonrpc"
	"gitlab.com/accumulatenetwork/accumulate/pkg/api/v3/message"
	"gitlab.com/accumulatenetwork/accumulate/pkg/api/v3/p2p"
	"gitlab.com/accumulatenetwork/accumulate/pkg/consensus"
	"gitlab.com/accumulatenetwork/accumulate/pkg/consensus/metrics"
	"gitlab.com/accumulatenetwork/accumulate/pkg/consensus/types"
	"gitlab.com/accumulatenetwork/accumulate/pkg/consensus/worker"
	"gitlab.com/accumulatenetwork/accumulate/pkg/types/messaging"
	"gitlab.com/accumulatenetwork/accumulate/pkg/url"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

// onePartition is the router a single-partition node has: everything this
// network carries belongs to the one partition. The relay's target choice is
// not routing and does not go through it.
type onePartition string

func (p onePartition) RouteAccount(*url.URL) (string, error)        { return string(p), nil }
func (p onePartition) Route(...*messaging.Envelope) (string, error) { return string(p), nil }

type relayNode struct {
	name    string
	pub     ed25519.PublicKey
	chost   host.Host
	cn      *consensus.Node
	svc     *Service
	apiNode *p2p.Node
	sub     *SubmitterService
	relay   *Relay
	joining bool

	mu        sync.Mutex
	committed map[string]bool
	blocks    int
}

// TestARelayCarriesWhatItCannotPropose is #4366's regression test: the
// debugger's five-node reproduction (note_3869694995) with its assertions
// inverted, driven through the callers rather than through the library.
//
// Six real pkg/consensus nodes on one partition: four in the committee, one
// of those still joining, and two whose keys are in no committee at all.
// Each also runs a real p2p.Node with its real submit and consensus service
// handlers installed, on its own libp2p host — so a relay is a real dial to
// a real peer's real handler, and the assertion is the transaction in a
// committed block on the validators.
//
// Before the fix the off-committee node accepted everything and committed
// none of it: its header is dropped before any vote
// (pkg/consensus/primary/vote_handler.go:277-284), and 1,249 of 1,249
// transactions vanished silently.
//
// What this test does by hand: the network definition (no chain here to read
// one from), the two libp2p meshes, genesis, and the commit loop a running
// node's executor would be. What is production: the client, the HTTP API
// handler and its network client, the dialer, the submit and consensus
// handlers at both ends, the relay's target choice, the counters, and the
// consensus that commits it.
func TestARelayCarriesWhatItCannotPropose(t *testing.T) {
	if testing.Short() {
		t.Skip("six consensus nodes, twelve libp2p hosts and half a minute of consensus")
	}

	const (
		nVals = 4
		// Unique, so the shared metric registry is ours — and MIXED CASE on
		// purpose: the harness reads the partition label as a key and joins
		// the three families on it, so none of them may fold the case.
		part    = "BVN3relay"
		netName = "RelayNet"
		runFor  = 12 * time.Second
	)
	// nVals validators (index nVals-1 is joining), then two followers.
	const (
		iJoining = nVals - 1
		iFol1    = nVals
		iFol2    = nVals + 1
		nNodes   = nVals + 2
	)

	ctx0 := context.Background()

	keys := make([]ed25519.PrivateKey, nNodes)
	pubs := make([]ed25519.PublicKey, nNodes)
	infos := make([]types.ValidatorInfo, nVals)
	for i := range keys {
		pub, priv, err := ed25519.GenerateKey(rand.Reader)
		require.NoError(t, err)
		keys[i], pubs[i] = priv, pub
		if i < nVals {
			infos[i] = types.ValidatorInfo{PublicKey: pub, Stake: 100}
		}
	}
	committee := types.NewCommittee(infos, 1)

	// The committee as the globals on chain give it: the four validators
	// active on the partition, the two followers in it nowhere.
	globals := globalsWith(t, map[string][]ed25519.PublicKey{
		part: {pubs[0], pubs[1], pubs[2], pubs[3]},
	})

	// The consensus mesh. GossipSub is created on every host BEFORE the
	// hosts are connected, so that the subscriptions travel with the
	// connection: a topic mesh that never forms produces headers and no
	// votes, and the test then measures nothing.
	chosts := make([]host.Host, nNodes)
	gossips := make([]*pubsub.PubSub, nNodes)
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

	nodes := make([]*relayNode, nNodes)
	var apiAddrs []multiaddr.Multiaddr
	for i := range nodes {
		ps := gossips[i]

		nodeCfg := consensus.NodeConfig{
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
		}
		cn, err := consensus.NewNode(nodeCfg, committee, chosts[i], ps)
		require.NoError(t, err)

		svc, err := NewService(ServiceConfig{
			Partition:  &protocol.PartitionInfo{ID: part, Type: protocol.PartitionTypeBlockValidator},
			NodeConfig: nodeCfg,
			Adapter:    &collectingAdapter{commitAdapter: commitAdapter{hash: [32]byte{0xAA}}},
			EventBus:   events.NewBus(nil),
		})
		require.NoError(t, err)
		svc.node = cn
		svc.committee = committee
		svc.ctx = ctx0

		// The API host. Each bootstraps to every host before it, so every
		// node is connected to every other and libp2p identify tells each
		// which services the others handle — which is how the dialer finds
		// a provider (connectedPeersDiscoverer), advertised or not.
		apiNode, err := p2p.New(p2p.Options{
			Network:        netName,
			Listen:         []multiaddr.Multiaddr{multiaddr.StringCast("/ip4/127.0.0.1/tcp/0")},
			BootstrapPeers: apiAddrs,
			DiscoveryMode:  dht.ModeServer,
		})
		require.NoError(t, err)
		t.Cleanup(func() { _ = apiNode.Close() })
		apiAddrs = append(apiAddrs, apiNode.Addresses()...)

		name := fmt.Sprintf("val%d", i)
		switch i {
		case iJoining:
			name = "joining-validator"
		case iFol1, iFol2:
			name = fmt.Sprintf("follower%d", i-nVals+1)
		}

		// Everything below is what cmd/accumulated/run/dagbft.go builds:
		// the membership predicate seeded from the globals, the relay over
		// this node's own p2p node and network client, and the two service
		// handlers registered for the partition.
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

		var state nodestate.Serving
		if i == iJoining {
			state = nodestate.New(protocol.PartitionUrl(part))
			require.False(t, state.CanServeCurrent())
		}

		sub := NewSubmitterService(SubmitterServiceParams{
			Service: svc, NodeState: state, Membership: m, Relay: relay,
		})
		cons := NewConsensusAPIService(ConsensusAPIServiceParams{
			Service:          svc,
			PartitionID:      part,
			PartitionType:    protocol.PartitionTypeBlockValidator,
			ValidatorKeyHash: sha256.Sum256(pubs[i]),
			NodeState:        state,
		})

		registerService(t, apiNode, api.ServiceTypeSubmit.AddressFor(part), message.Submitter{Submitter: sub})
		registerService(t, apiNode, api.ServiceTypeConsensus.AddressFor(part), message.ConsensusService{ConsensusService: cons})

		nodes[i] = &relayNode{
			name: name, pub: pubs[i], chost: chosts[i], cn: cn, svc: svc,
			apiNode: apiNode, sub: sub, relay: relay, joining: i == iJoining,
			committed: map[string]bool{},
		}
	}

	for _, nd := range nodes {
		require.NoError(t, nd.cn.InsertGenesisForAll(keys[:nVals]))
	}

	ctx, cancel := context.WithCancel(ctx0)
	var wg sync.WaitGroup
	for _, nd := range nodes {
		nd := nd
		wg.Add(1)
		go func() { defer wg.Done(); _ = nd.cn.Start(ctx) }()
		wg.Add(1)
		go func() {
			defer wg.Done()
			for {
				select {
				case <-ctx.Done():
					return
				case group, ok := <-nd.cn.Committed():
					if !ok {
						return
					}
					for _, cert := range group {
						batches, err := nd.cn.CollectBatches(ctx, cert)
						if err != nil {
							continue
						}
						nd.mu.Lock()
						for _, b := range batches {
							for _, tx := range b.Transactions {
								nd.committed[string(tx)] = true
							}
						}
						nd.blocks++
						nd.mu.Unlock()
						digests := make([]types.BatchDigest, 0, len(cert.Header.Payload))
						for _, e := range cert.Header.Payload {
							digests = append(digests, e.Digest)
						}
						for _, w := range nd.cn.Workers() {
							w.PruneCommitted(digests, worker.CommitInfo{Detail: "test", Cert: cert.Digest().String()})
						}
					}
					nd.cn.ReportExecuted()
				}
			}
		}()
	}
	defer func() {
		cancel()
		wg.Wait()
		for _, nd := range nodes {
			nd.cn.Stop()
		}
	}()

	// The API mesh has to have exchanged identify before a relay can find a
	// provider that is not itself.
	require.Eventually(t, func() bool {
		return len(nodes[iFol1].apiNode.Providers(ctx, api.ServiceTypeSubmit.AddressFor(part), 10)) >= nNodes-1
	}, 30*time.Second, 200*time.Millisecond, "the API mesh never formed")
	time.Sleep(2 * time.Second) // the consensus mesh

	// The follower's own HTTP API, wired exactly as the daemon wires it:
	// JSON-RPC Submitter -> the NETWORK client -> this node's DialNetwork.
	// That dial is local-first, so it lands on this node's own submit
	// service, which is where the relay is (internal/node/http/handler.go:81-107).
	h, err := nodehttp.NewHandler(nodehttp.Options{
		Node:      nodes[iFol1].apiNode,
		Router:    onePartition(part),
		NetworkId: netName,
	})
	require.NoError(t, err)
	httpSrv := httptest.NewServer(h)
	defer httpSrv.Close()
	userClient := jsonrpc.NewClient(httpSrv.URL + "/v3")

	// A peer with no services of its own, dialling the follower's submit
	// service the way a validator's dispatcher does.
	peerNode, err := p2p.New(p2p.Options{
		Network:        netName,
		Listen:         []multiaddr.Multiaddr{multiaddr.StringCast("/ip4/127.0.0.1/tcp/0")},
		BootstrapPeers: apiAddrs,
		DiscoveryMode:  dht.ModeServer,
	})
	require.NoError(t, err)
	defer func() { _ = peerNode.Close() }()
	peerClient := &message.Client{Transport: &message.RoutedTransport{
		Network: netName,
		Dialer:  peerNode.DialNetwork(),
	}}

	verify := false
	opts := api.SubmitOptions{Verify: &verify}
	mark := func(prefix string, i int) *messaging.Envelope {
		env := new(messaging.Envelope)
		env.TxHash = []byte(fmt.Sprintf("%s%06d", prefix, i))
		return env
	}

	a0, r0 := submissionCounts(part)
	taken0 := testutil.ToFloat64(metrics.RelayedTotal.WithLabelValues(part, metrics.RelayTaken))
	certified0 := testutil.ToFloat64(metrics.CertifiedOwnTransactionsTotal.WithLabelValues(part))

	var nHTTP, nPeer, nJoin, nVal int
	var failed []string
	stop := time.After(runFor)
	tick := time.NewTicker(60 * time.Millisecond)
	defer tick.Stop()
	round := 0
loop:
	for {
		select {
		case <-stop:
			break loop
		case <-tick.C:
			round++

			// (a) a client at the follower's real HTTP API.
			nHTTP++
			if _, err := userClient.Submit(ctx, mark("HTTP-TX-", round), opts); err != nil {
				failed = append(failed, fmt.Sprintf("http %d: %v", round, err))
			}

			// (b) a peer dialling the follower's submit service directly,
			// which is the shape of a validator's dispatcher and of the
			// router that picked the follower as a provider.
			nPeer++
			if _, err := peerClient.ForPeer(nodes[iFol1].apiNode.ID()).
				ForAddress(api.ServiceTypeSubmit.AddressFor(part).Multiaddr()).
				Submit(ctx, mark("PEER-TX-", round), opts); err != nil {
				failed = append(failed, fmt.Sprintf("peer %d: %v", round, err))
			}

			// (c) the same, into a node that is in the committee but still
			// joining: a relay needs no local state, so it relays too.
			nJoin++
			if _, err := peerClient.ForPeer(nodes[iJoining].apiNode.ID()).
				ForAddress(api.ServiceTypeSubmit.AddressFor(part).Multiaddr()).
				Submit(ctx, mark("JOIN-TX-", round), opts); err != nil {
				failed = append(failed, fmt.Sprintf("joining %d: %v", round, err))
			}

			// A validator's own path, for comparison.
			nVal++
			if _, err := peerClient.ForPeer(nodes[0].apiNode.ID()).
				ForAddress(api.ServiceTypeSubmit.AddressFor(part).Multiaddr()).
				Submit(ctx, mark("VAL-TX-", round), opts); err != nil {
				failed = append(failed, fmt.Sprintf("validator %d: %v", round, err))
			}
		}
	}
	time.Sleep(5 * time.Second) // drain

	require.Empty(t, failed, "a submission a node could not propose was refused instead of relayed")

	// What reached a block, anywhere.
	seen := map[string]int{}
	for _, nd := range nodes {
		nd.mu.Lock()
		counts := map[string]int{}
		for tx := range nd.committed {
			for _, p := range []string{"HTTP-TX-", "PEER-TX-", "JOIN-TX-", "VAL-TX-"} {
				if strings.Contains(tx, p) {
					counts[p]++
				}
			}
		}
		t.Logf("%-18s blocks=%d committed=%d http=%d peer=%d joining=%d validator=%d",
			nd.name, nd.blocks, len(nd.committed),
			counts["HTTP-TX-"], counts["PEER-TX-"], counts["JOIN-TX-"], counts["VAL-TX-"])
		for k, v := range counts {
			if v > seen[k] {
				seen[k] = v
			}
		}
		nd.mu.Unlock()
	}

	require.NotZero(t, seen["VAL-TX-"], "the network did not run")
	require.NotZero(t, seen["HTTP-TX-"],
		"a transaction submitted to the follower's own API never reached a block (#4366)")
	require.NotZero(t, seen["PEER-TX-"],
		"a transaction a peer dialled to the follower never reached a block (#4366)")
	require.NotZero(t, seen["JOIN-TX-"],
		"a transaction submitted to a joining node never reached a block")

	// It follows, or this proves nothing.
	for _, i := range []int{iFol1, iFol2} {
		nodes[i].mu.Lock()
		b := nodes[i].blocks
		nodes[i].mu.Unlock()
		require.NotZero(t, b, "%s executed no block at all", nodes[i].name)
	}

	// The three counter families, read off the registry the exporter serves.
	a1, r1 := submissionCounts(part)
	taken := testutil.ToFloat64(metrics.RelayedTotal.WithLabelValues(part, metrics.RelayTaken)) - taken0
	certified := testutil.ToFloat64(metrics.CertifiedOwnTransactionsTotal.WithLabelValues(part)) - certified0
	relayedAll := float64(0)
	for _, o := range []string{metrics.RelayTaken, metrics.RelayRefused, metrics.RelayNotReady, metrics.RelayUnreachable} {
		relayedAll += testutil.ToFloat64(metrics.RelayedTotal.WithLabelValues(part, o))
	}
	t.Logf("accepted=%.0f rejected=%.0f relayed-taken=%.0f relayed-all=%.0f certified_own=%.0f (sent http=%d peer=%d joining=%d validator=%d)",
		a1-a0, r1-r0, taken, relayedAll, certified, nHTTP, nPeer, nJoin, nVal)

	relayed := float64(nHTTP + nPeer + nJoin)
	require.Equal(t, relayed, taken,
		"ONE hop per submission: a second follower relaying it again would count it twice")
	require.Equal(t, float64(0), relayedAll-taken, "no submission ended refused, not-ready or unreachable")
	require.NotZero(t, certified, "the validators certified their own transactions")

	// Every relayed submission is counted accepted twice across this
	// registry — once by the node that relayed it and once by the validator
	// that took it — because the harness reads the families PER NODE and
	// both nodes did return success. Plus the validator's own traffic.
	require.Equal(t, 2*relayed+float64(nVal), a1-a0,
		"accepted is what each node returned success for")
	require.Equal(t, float64(0), r1-r0, "nothing was rejected")

	// The label is the partition ID as the node holds it, in all three
	// families: fold the case and the harness's join produces half-rows.
	low := strings.ToLower(part)
	require.Zero(t, testutil.ToFloat64(metrics.SubmissionsTotal.WithLabelValues(low, "accepted")))
	require.Zero(t, testutil.ToFloat64(metrics.CertifiedOwnTransactionsTotal.WithLabelValues(low)))
	require.Zero(t, testutil.ToFloat64(metrics.RelayedTotal.WithLabelValues(low, metrics.RelayTaken)))
}

func submissionCounts(part string) (accepted, rejected float64) {
	return testutil.ToFloat64(metrics.SubmissionsTotal.WithLabelValues(part, "accepted")),
		testutil.ToFloat64(metrics.SubmissionsTotal.WithLabelValues(part, "rejected"))
}

func registerService(t *testing.T, n *p2p.Node, sa *api.ServiceAddress, svc message.Service) {
	t.Helper()
	h, err := message.NewHandler(svc)
	require.NoError(t, err)
	require.True(t, n.RegisterService(sa, h.Handle))
}
