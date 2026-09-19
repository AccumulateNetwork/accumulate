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
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/libp2p/go-libp2p"
	pubsub "github.com/libp2p/go-libp2p-pubsub"
	"github.com/libp2p/go-libp2p/core/host"
	"github.com/libp2p/go-libp2p/core/peer"
	"github.com/prometheus/client_golang/prometheus/testutil"
	"github.com/stretchr/testify/require"
	"gitlab.com/accumulatenetwork/accumulate/internal/core/events"
	"gitlab.com/accumulatenetwork/accumulate/pkg/api/v3"
	"gitlab.com/accumulatenetwork/accumulate/pkg/consensus"
	"gitlab.com/accumulatenetwork/accumulate/pkg/consensus/metrics"
	"gitlab.com/accumulatenetwork/accumulate/pkg/consensus/types"
	"gitlab.com/accumulatenetwork/accumulate/pkg/consensus/worker"
	"gitlab.com/accumulatenetwork/accumulate/pkg/errors"
	"gitlab.com/accumulatenetwork/accumulate/pkg/types/messaging"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

// TestFollowerRefusesAndNothingStrands is the debugger's reproduction of
// #4366 turned into the regression test for its fix (note_3869695902).
//
// Five real pkg/consensus nodes on one partition over loopback libp2p, four
// of them the committee and the fifth's key in no committee — the soak's
// follower. Before the fix the follower accepted 1,249 of 1,249 submissions,
// authored 395 headers, created 0 certificates, and not one of those
// transactions reached any node's committed block: validators drop a header
// whose author is outside the committee before they vote on it
// (pkg/consensus/primary/vote_handler.go:277-284).
//
// After it, every submission to the follower is refused with NotReady and
// nothing is taken to strand, while the four validators accept and commit as
// before and the follower executes every block the committee makes.
//
// What this test does by hand: the network definition (there is no chain
// here to read one from), the libp2p mesh, genesis, and the commit loop that
// a running node's executor would be. What is production: the submission
// path — SubmitterService.Submit, its Membership predicate, Service and the
// consensus node beneath it — and the counters, read from the registry the
// exporter serves.
func TestFollowerRefusesAndNothingStrands(t *testing.T) {
	if testing.Short() {
		t.Skip("five libp2p nodes and twelve seconds of consensus")
	}

	const (
		nVals  = 4
		part   = "bvn3strand" // unique, so the shared metric registry is ours
		runFor = 10 * time.Second
	)

	keys := make([]ed25519.PrivateKey, nVals+1)
	pubs := make([]ed25519.PublicKey, nVals+1)
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

	// The network definition every node reads its committee from: the four
	// validators active on the partition, the follower in it nowhere. This
	// is what a node loads from the globals on chain.
	globals := globalsWith(t, map[string][]ed25519.PublicKey{
		part: {pubs[0], pubs[1], pubs[2], pubs[3]},
	})

	ctx0 := context.Background()
	hosts := make([]host.Host, nVals+1)
	for i := range hosts {
		h, err := libp2p.New(libp2p.ListenAddrStrings("/ip4/127.0.0.1/tcp/0"))
		require.NoError(t, err)
		hosts[i] = h
		defer h.Close() //nolint:gocritic // one per node, for the test's life
	}
	for i := range hosts {
		for j := i + 1; j < len(hosts); j++ {
			require.NoError(t, hosts[i].Connect(ctx0, peer.AddrInfo{ID: hosts[j].ID(), Addrs: hosts[j].Addrs()}))
		}
	}

	type node struct {
		name string
		n    *consensus.Node
		sub  *SubmitterService

		mu        sync.Mutex
		committed map[string]bool
		blocks    int
	}

	nodes := make([]*node, nVals+1)
	for i := range nodes {
		ps, err := pubsub.NewGossipSub(ctx0, hosts[i])
		require.NoError(t, err)

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
		cn, err := consensus.NewNode(nodeCfg, committee, hosts[i], ps)
		require.NoError(t, err)

		// The service and the submitter the daemon builds, with the
		// membership predicate the daemon gives them.
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

		m := NewMembership(part, pubs[i])
		m.SetGlobals(globals)

		name := fmt.Sprintf("val%d", i)
		if i == nVals {
			name = "follower"
			require.False(t, m.InCommittee(), "the fifth node must be in no committee")
		} else {
			require.True(t, m.InCommittee())
		}

		nodes[i] = &node{
			name:      name,
			n:         cn,
			sub:       NewSubmitterService(SubmitterServiceParams{Service: svc, Membership: m}),
			committed: map[string]bool{},
		}
	}

	for _, nd := range nodes {
		require.NoError(t, nd.n.InsertGenesisForAll(keys[:nVals]))
	}

	ctx, cancel := context.WithCancel(ctx0)
	var wg sync.WaitGroup
	for _, nd := range nodes {
		nd := nd
		wg.Add(1)
		go func() { defer wg.Done(); _ = nd.n.Start(ctx) }()
		wg.Add(1)
		go func() {
			defer wg.Done()
			for {
				select {
				case <-ctx.Done():
					return
				case group, ok := <-nd.n.Committed():
					if !ok {
						return
					}
					for _, cert := range group {
						batches, err := nd.n.CollectBatches(ctx, cert)
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
						for _, w := range nd.n.Workers() {
							w.PruneCommitted(digests, worker.CommitInfo{Detail: "test", Cert: cert.Digest().String()})
						}
					}
					nd.n.ReportExecuted()
				}
			}
		}()
	}
	defer func() {
		cancel()
		wg.Wait()
		for _, nd := range nodes {
			nd.n.Stop()
		}
	}()

	time.Sleep(2 * time.Second) // mesh

	accepted0 := testutil.ToFloat64(metrics.SubmissionsTotal.WithLabelValues(part, "accepted"))
	rejected0 := testutil.ToFloat64(metrics.SubmissionsTotal.WithLabelValues(part, "rejected"))
	certified0 := testutil.ToFloat64(metrics.CertifiedOwnTransactionsTotal.WithLabelValues(part))

	// Both nodes are handed the same traffic through the same entry point:
	// the API's Submit. Verification is off because these envelopes carry no
	// signatures — what is being measured is what the node does with what it
	// takes, not what the executor makes of it.
	verify := false
	opts := api.SubmitOptions{Verify: &verify}
	mark := func(prefix string, i int) *messaging.Envelope {
		env := new(messaging.Envelope)
		env.TxHash = []byte(fmt.Sprintf("%s%06d", prefix, i))
		return env
	}

	var sent, refusedVal, refusedFol int
	stop := time.After(runFor)
	tick := time.NewTicker(20 * time.Millisecond)
	defer tick.Stop()
loop:
	for {
		select {
		case <-stop:
			break loop
		case <-tick.C:
			sent++
			if _, err := nodes[0].sub.Submit(ctx, mark("VAL-TX-", sent), opts); err != nil {
				refusedVal++
			}
			_, err := nodes[nVals].sub.Submit(ctx, mark("FOL-TX-", sent), opts)
			if err == nil {
				t.Fatalf("the follower ACCEPTED a submission it can never propose (#4366)")
			}
			require.Truef(t, errors.Is(err, errors.NotReady), "the follower must refuse with NotReady: %v", err)
			require.Contains(t, err.Error(), "committee")
			refusedFol++
		}
	}
	time.Sleep(4 * time.Second) // drain

	var valSeen, folSeen int
	for _, nd := range nodes {
		nd.mu.Lock()
		v, f := 0, 0
		for tx := range nd.committed {
			switch {
			case strings.Contains(tx, "VAL-TX-"):
				v++
			case strings.Contains(tx, "FOL-TX-"):
				f++
			}
		}
		t.Logf("%-9s blocks=%d committed=%d from-validator=%d from-follower=%d",
			nd.name, nd.blocks, len(nd.committed), v, f)
		valSeen += v
		folSeen += f
		nd.mu.Unlock()
	}
	t.Logf("offered %d to the validator (%d refused), %d to the follower (%d refused)",
		sent, refusedVal, sent, refusedFol)

	require.Equal(t, sent, refusedFol, "the follower must refuse every submission")
	require.NotZero(t, valSeen, "the validator's transactions never committed — the network did not run")
	require.Zero(t, folSeen, "a transaction the follower took reached a block, which cannot happen")

	nodes[nVals].mu.Lock()
	fb := nodes[nVals].blocks
	nodes[nVals].mu.Unlock()
	require.NotZero(t, fb, "the follower executed no block at all; it must follow, or this proves nothing")

	// The counters the harness reads. They are process-wide, so these are
	// the five nodes together on this partition: what separates the follower
	// is that it contributed only rejections.
	accepted := testutil.ToFloat64(metrics.SubmissionsTotal.WithLabelValues(part, "accepted")) - accepted0
	rejected := testutil.ToFloat64(metrics.SubmissionsTotal.WithLabelValues(part, "rejected")) - rejected0
	certified := testutil.ToFloat64(metrics.CertifiedOwnTransactionsTotal.WithLabelValues(part)) - certified0
	t.Logf("accumulate_dagbft_submissions_total accepted=%.0f rejected=%.0f, certified_own=%.0f",
		accepted, rejected, certified)
	require.GreaterOrEqual(t, rejected, float64(refusedFol), "every refusal is counted")
	require.Equal(t, float64(sent-refusedVal), accepted, "every acceptance is counted, and only the validator's")
	require.NotZero(t, certified, "the validators certified their own transactions")
	require.LessOrEqual(t, certified, accepted,
		"certified must never exceed accepted — the harness reads that as an instrument alarm")
}
