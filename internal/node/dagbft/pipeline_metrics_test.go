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
	"testing"
	"time"

	"github.com/libp2p/go-libp2p"
	pubsub "github.com/libp2p/go-libp2p-pubsub"
	"github.com/libp2p/go-libp2p/core/host"
	"github.com/libp2p/go-libp2p/core/peer"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/testutil"
	"github.com/stretchr/testify/require"
	"gitlab.com/accumulatenetwork/accumulate/internal/core/events"
	"gitlab.com/accumulatenetwork/accumulate/internal/database"
	"gitlab.com/accumulatenetwork/accumulate/pkg/consensus"
	"gitlab.com/accumulatenetwork/accumulate/pkg/consensus/adapter"
	"gitlab.com/accumulatenetwork/accumulate/pkg/consensus/metrics"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

// The pipeline metrics move when blocks are produced (#4181). Every stage's
// metric was registered, scraped and permanently zero because nothing wrote
// it, so a wedged partition could not be told from a healthy one by its
// metrics. Four validators run the production Service over libp2p; once they
// have produced blocks, each stage's collector must have moved. The
// collectors are read as a scrape would read them, not by calling the
// setters. They are process-wide, so the test asserts positive, never exact.
func TestPipelineMetricsMoveWhenBlocksAreProduced(t *testing.T) {
	if testing.Short() {
		t.Skip("four consensus nodes over libp2p")
	}

	const (
		nVals = 4
		part  = "BVN5metrics"
	)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	keys := make([]ed25519.PrivateKey, nVals)
	initial := make([]adapter.ValidatorInfo, nVals)
	for i := range keys {
		pub, priv, err := ed25519.GenerateKey(rand.Reader)
		require.NoError(t, err)
		keys[i] = priv
		initial[i] = adapter.ValidatorInfo{PublicKey: [32]byte(pub), Stake: 100, Active: true}
	}

	hosts := make([]host.Host, nVals)
	gossips := make([]*pubsub.PubSub, nVals)
	for i := range hosts {
		h, err := libp2p.New(libp2p.ListenAddrStrings("/ip4/127.0.0.1/tcp/0"))
		require.NoError(t, err)
		hosts[i] = h
		t.Cleanup(func() { _ = h.Close() })
		gossips[i], err = pubsub.NewGossipSub(ctx, h)
		require.NoError(t, err)
	}
	for i := range hosts {
		for j := i + 1; j < len(hosts); j++ {
			require.NoError(t, hosts[i].Connect(ctx, peer.AddrInfo{ID: hosts[j].ID(), Addrs: hosts[j].Addrs()}))
		}
	}

	vals := make([]*Service, nVals)
	for i := range vals {
		svc, err := NewService(ServiceConfig{
			Partition: &protocol.PartitionInfo{ID: part, Type: protocol.PartitionTypeBlockValidator},
			NodeConfig: consensus.NodeConfig{
				Partition:           part,
				KeyPair:             keys[i],
				NumWorkers:          1,
				MinRoundInterval:    50 * time.Millisecond,
				BatchCollectTimeout: time.Minute,
			},
			Adapter:           &commitAdapter{},
			EventBus:          events.NewBus(nil),
			Host:              hosts[i],
			PubSub:            gossips[i],
			InitialValidators: initial,
			Database:          database.OpenInMemory(nil),
		})
		require.NoError(t, err)
		require.NoError(t, svc.Start(ctx))
		t.Cleanup(func() { _ = svc.Stop() })
		vals[i] = svc
	}

	go func() {
		for n := 0; ctx.Err() == nil; n++ {
			_ = vals[n%nVals].SubmitTransaction([]byte(fmt.Sprintf("tx-%d", n)))
			time.Sleep(10 * time.Millisecond)
		}
	}()

	// Blocks are the precondition: the counter that always worked.
	blocks := metrics.BlocksProducedTotal.WithLabelValues(part)
	require.Eventually(t, func() bool { return testutil.ToFloat64(blocks) > 0 },
		time.Minute, 50*time.Millisecond, "the validators produced no block")

	for _, c := range []struct {
		name string
		c    prometheus.Collector
	}{
		{"current_round", metrics.CurrentRound.WithLabelValues(part)},
		{"last_commit_round", metrics.LastCommitRound.WithLabelValues(part)},
		{"headers_created_total", metrics.HeadersCreatedTotal.WithLabelValues(part)},
		{"votes_received_total", metrics.VotesReceivedTotal.WithLabelValues(part)},
		{"votes_sent_total", metrics.VotesSentTotal.WithLabelValues(part)},
		{"certificates_created_total", metrics.CertificatesCreatedTotal.WithLabelValues(part)},
		{"certificates_committed_total", metrics.CertificatesCommittedTotal.WithLabelValues(part)},
		{"leader_commits_total", metrics.LeaderCommitsTotal.WithLabelValues(part)},
		{"dag_rounds", metrics.DAGRounds.WithLabelValues(part)},
		{"dag_certificates", metrics.DAGCertificates.WithLabelValues(part)},
		{"peers_connected", metrics.PeersConnected.WithLabelValues(part)},
	} {
		c := c
		require.Eventually(t, func() bool { return testutil.ToFloat64(c.c) > 0 },
			10*time.Second, 50*time.Millisecond, "accumulate_dagbft_%s is still zero after blocks were produced", c.name)
	}
}
