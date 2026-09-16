// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package e2e

import (
	"fmt"
	"math/big"
	"math/rand"
	"testing"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/stretchr/testify/require"
	"gitlab.com/accumulatenetwork/accumulate/pkg/build"
	"gitlab.com/accumulatenetwork/accumulate/pkg/url"
	. "gitlab.com/accumulatenetwork/accumulate/protocol"
	. "gitlab.com/accumulatenetwork/accumulate/test/harness"
	. "gitlab.com/accumulatenetwork/accumulate/test/helpers"
	"gitlab.com/accumulatenetwork/accumulate/test/simulator"
	acctesting "gitlab.com/accumulatenetwork/accumulate/test/testing"
)

// The determinism gate #4149 asks for before ExecutionShards leaves 1: one
// network whose nodes run at 1, 8 and 64 shards -- 8 being the count a soak
// runs at -- under randomized traffic that mixes every shape classification
// has to reason about, for enough blocks that shard-order effects would
// surface. The simulator compares every node's deliver and commit results on
// every block, so a divergence fails the step it happens on.
//
// Shapes, chosen per identity per round from a seeded generator so a failure
// is reproducible: an intra-identity transfer (always shardable), a transfer
// to a random other identity (a produced deposit, same or cross partition), a
// data write, and a HELD transfer (serial). Two identities run a 2-of-2 page
// whose second signature arrives in the NEXT round as a signature-only
// envelope (resolved from the store, not from a claim), and one identity is
// signed for cross-ADI by a delegate (a genuine two-identity envelope).
//
// The test asserts its own coverage: the parallel lane must actually have
// executed on the sharded nodes, or the comparison was serial against serial
// and proves nothing (the vacuity the first gate had until #4149).
func TestShardEquivalence_Randomized(t *testing.T) {
	const identities = 16
	rounds := 8
	if testing.Short() {
		rounds = 3
	}
	rng := rand.New(rand.NewSource(4149))

	sim := NewSim(t,
		simulator.SimpleNetwork("ShardRand", 3, 3),
		simulator.Genesis(GenesisTime),
		simulator.ExecutionShardsPerNode(1, 8, 64),
	)

	type party struct {
		id   *url.URL
		keys [][]byte
		ts   uint64
	}
	parties := make([]*party, identities)
	for i := range parties {
		id := AccountUrl(fmt.Sprintf("rand-%02d", i))
		p := &party{id: id}
		nkeys := 1
		if i < 2 {
			nkeys = 2 // 2-of-2 pages
		}
		var pub [][]byte
		for k := 0; k < nkeys; k++ {
			key := acctesting.GenerateKey(id, k)
			p.keys = append(p.keys, key)
			pub = append(pub, key[32:])
		}
		MakeIdentity(t, sim.DatabaseFor(id), id, pub...)
		CreditCredits(t, sim.DatabaseFor(id), id.JoinPath("book", "1"), 1e9)
		MakeAccount(t, sim.DatabaseFor(id),
			&TokenAccount{Url: id.JoinPath("tokens"), TokenUrl: AcmeUrl()},
			&TokenAccount{Url: id.JoinPath("savings"), TokenUrl: AcmeUrl()},
			&DataAccount{Url: id.JoinPath("data")})
		CreditTokens(t, sim.DatabaseFor(id), id.JoinPath("tokens"), big.NewInt(1e12))
		parties[i] = p
	}
	for _, p := range parties[:2] {
		UpdateAccount(t, sim.DatabaseFor(p.id), p.id.JoinPath("book", "1"), func(pg *KeyPage) { pg.AcceptThreshold = 2 })
	}
	// parties[3] delegates to parties[4]'s book: 4 signs for 3, cross-ADI.
	delegator, delegate := parties[3], parties[4]
	UpdateAccount(t, sim.DatabaseFor(delegator.id), delegator.id.JoinPath("book", "1"), func(pg *KeyPage) {
		pg.AddKeySpec(&KeySpec{Delegate: delegate.id.JoinPath("book")})
		require.NoError(t, pg.SetThreshold(1))
	})
	next := func(p *party) uint64 { p.ts++; return p.ts }
	page := func(p *party) *url.URL { return p.id.JoinPath("book", "1") }

	flushesBefore := gaugeSum(t, "accumulate_exec_flushes_total")

	var pendingMultisig []struct {
		p    *party
		txid *url.TxID
	}
	for r := 0; r < rounds; r++ {
		var complete []*url.TxID // completes this round
		var later []*url.TxID    // completes after a hold or a second signature

		// The second signature for last round's 2-of-2 initiations: a
		// signature-only envelope whose transaction must be resolved from
		// the store.
		for _, ms := range pendingMultisig {
			env := MustBuild(t, build.SignatureForTxID(ms.txid).
				Url(page(ms.p)).Version(1).Timestamp(next(ms.p)).PrivateKey(ms.p.keys[1]))
			sim.SubmitSuccessfully(env)
			later = append(later, ms.txid)
		}
		pendingMultisig = pendingMultisig[:0]

		for i, p := range parties {
			switch {
			case i < 2:
				// 2-of-2: initiate with key 1, complete next round with key 2.
				st := sim.SubmitTxnSuccessfully(MustBuild(t,
					build.Transaction().For(p.id.JoinPath("tokens")).
						SendTokens(1, 0).To(p.id.JoinPath("savings")).
						SignWith(page(p)).Version(1).Timestamp(next(p)).PrivateKey(p.keys[0])))
				pendingMultisig = append(pendingMultisig, struct {
					p    *party
					txid *url.TxID
				}{p, st.TxID})
				continue
			case p == delegator:
				// Signed by the delegate, cross-ADI: a two-identity envelope.
				st := sim.SubmitTxnSuccessfully(MustBuild(t,
					build.Transaction().For(p.id.JoinPath("tokens")).
						SendTokens(1, 0).To(p.id.JoinPath("savings")).
						SignWith(page(delegate)).Delegator(page(p)).
						Version(1).Timestamp(next(delegate)).PrivateKey(delegate.keys[0])))
				complete = append(complete, st.TxID)
				continue
			}
			switch rng.Intn(4) {
			case 0: // intra-identity
				st := sim.SubmitTxnSuccessfully(MustBuild(t,
					build.Transaction().For(p.id.JoinPath("tokens")).
						SendTokens(1, 0).To(p.id.JoinPath("savings")).
						SignWith(page(p)).Version(1).Timestamp(next(p)).PrivateKey(p.keys[0])))
				complete = append(complete, st.TxID)
			case 1: // to a random other identity: a produced deposit
				q := parties[rng.Intn(identities)]
				if q == p {
					q = parties[(i+1)%identities]
				}
				st := sim.SubmitTxnSuccessfully(MustBuild(t,
					build.Transaction().For(p.id.JoinPath("tokens")).
						SendTokens(2, 0).To(q.id.JoinPath("tokens")).
						SignWith(page(p)).Version(1).Timestamp(next(p)).PrivateKey(p.keys[0])))
				complete = append(complete, st.TxID)
			case 2: // a data write
				st := sim.SubmitTxnSuccessfully(MustBuild(t,
					build.Transaction().For(p.id.JoinPath("data")).
						WriteData().DoubleHash([]byte(fmt.Sprintf("round %d party %d", r, i))).
						SignWith(page(p)).Version(1).Timestamp(next(p)).PrivateKey(p.keys[0])))
				complete = append(complete, st.TxID)
			case 3: // held: serial, and it executes blocks later
				st := sim.SubmitTxnSuccessfully(MustBuild(t,
					build.Transaction().For(p.id.JoinPath("tokens")).
						HoldUntil(HoldUntilOptions{MinorBlock: sim.S.BlockIndex("Directory") + 6}).
						SendTokens(1, 0).To(p.id.JoinPath("savings")).
						SignWith(page(p)).Version(1).Timestamp(next(p)).PrivateKey(p.keys[0])))
				later = append(later, st.TxID)
			}
		}
		for _, id := range complete {
			sim.StepUntil(Txn(id).Completes())
		}
		for _, id := range later {
			sim.StepUntilN(120, Txn(id).Completes())
		}
	}
	// The tail: the last round's 2-of-2 initiations complete now.
	for _, ms := range pendingMultisig {
		sim.SubmitSuccessfully(MustBuild(t, build.SignatureForTxID(ms.txid).
			Url(page(ms.p)).Version(1).Timestamp(next(ms.p)).PrivateKey(ms.p.keys[1])))
		sim.StepUntilN(120, Txn(ms.txid).Completes())
	}
	// Let synthetics and anchors settle under comparison too.
	sim.StepN(30)

	// Coverage: the parallel lane executed on the sharded nodes.
	require.Greater(t, gaugeSum(t, "accumulate_exec_flushes_total"), flushesBefore,
		"no shard ever flushed a parallel run: the comparison was serial against serial and proves nothing")
}

// gaugeSum is the sum of a counter over every label set, read from the
// process's default registry -- the same numbers a node exports.
func gaugeSum(t *testing.T, name string) float64 {
	t.Helper()
	mfs, err := prometheus.DefaultGatherer.Gather()
	require.NoError(t, err)
	var sum float64
	for _, mf := range mfs {
		if mf.GetName() != name {
			continue
		}
		for _, m := range mf.GetMetric() {
			sum += m.GetCounter().GetValue() + m.GetGauge().GetValue()
		}
	}
	return sum
}
