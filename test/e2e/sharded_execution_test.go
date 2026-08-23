// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package e2e

import (
	"fmt"
	"math/big"
	"testing"

	"github.com/stretchr/testify/require"
	"gitlab.com/accumulatenetwork/accumulate/pkg/build"
	"gitlab.com/accumulatenetwork/accumulate/pkg/url"
	. "gitlab.com/accumulatenetwork/accumulate/protocol"
	. "gitlab.com/accumulatenetwork/accumulate/test/harness"
	. "gitlab.com/accumulatenetwork/accumulate/test/helpers"
	"gitlab.com/accumulatenetwork/accumulate/test/simulator"
	acctesting "gitlab.com/accumulatenetwork/accumulate/test/testing"
)

// #4145's gate: shard count is a local parallelism choice that cannot change
// the result. Everything else in the shard test plan is diagnosis for when
// this fails.
//
// HOW the check works matters. Comparing two separate simulations does not:
// anchor signatures are wall-clock-timestamped, so even two serial runs of
// identical traffic diverge (verified — that is a property of the harness,
// not of execution). Instead, ONE network runs with every node at a
// DIFFERENT shard count. All nodes execute the same blocks with the same
// inputs — including the same wall-clock-signed anchors — and the
// simulator's consensus compares every node's deliver and commit results on
// every block. If shard count could change any result, the step fails on the
// block where it does. The traffic below drives multi-envelope parallel runs
// (many identities sharing blocks), intra-identity transfers, cross-identity
// and cross-partition deposits, and the serial barriers synthetics and
// anchors create.
func TestShardCountDoesNotChangeBlockHash(t *testing.T) {
	const identities = 12
	const rounds = 3

	sim := NewSim(t,
		simulator.SimpleNetwork("Sharded", 2, 3),
		simulator.Genesis(GenesisTime),
		// Node 0 executes serially, node 1 at 4 shards, node 2 at 64 — on
		// every partition, for every block of this test.
		simulator.ExecutionShardsPerNode(1, 4, 64),
	)

	type party struct {
		key []byte
		id  *url.URL
	}
	parties := make([]party, identities)
	for i := range parties {
		id := AccountUrl(fmt.Sprintf("party-%d", i))
		key := acctesting.GenerateKey(id)
		MakeIdentity(t, sim.DatabaseFor(id), id, key[32:])
		CreditCredits(t, sim.DatabaseFor(id), id.JoinPath("book", "1"), 1e9)
		MakeAccount(t, sim.DatabaseFor(id),
			&TokenAccount{Url: id.JoinPath("tokens"), TokenUrl: AcmeUrl()},
			&TokenAccount{Url: id.JoinPath("savings"), TokenUrl: AcmeUrl()})
		CreditTokens(t, sim.DatabaseFor(id), id.JoinPath("tokens"), big.NewInt(1e12))
		parties[i] = party{key, id}
	}

	// Each round: every identity moves tokens to its own savings (pure
	// intra-identity, always shardable) and to the NEXT identity's tokens (a
	// produced deposit — same or cross partition depending on routing). All
	// of a round's transactions are submitted before stepping, so they share
	// blocks and form real multi-envelope parallel runs.
	for r := 0; r < rounds; r++ {
		// Timestamps are per-signer nonces and must increase monotonically:
		// two submissions per party per round.
		ts := uint64(2*r + 1)
		sts := make([]*TransactionStatus, 0, 2*identities)
		for i, p := range parties {
			st := sim.SubmitTxnSuccessfully(MustBuild(t,
				build.Transaction().For(p.id.JoinPath("tokens")).
					SendTokens(1, 0).To(p.id.JoinPath("savings")).
					SignWith(p.id.JoinPath("book", "1")).Version(1).Timestamp(ts).PrivateKey(p.key)))
			sts = append(sts, st)

			next := parties[(i+1)%identities]
			st = sim.SubmitTxnSuccessfully(MustBuild(t,
				build.Transaction().For(p.id.JoinPath("tokens")).
					SendTokens(2, 0).To(next.id.JoinPath("tokens")).
					SignWith(p.id.JoinPath("book", "1")).Version(1).Timestamp(ts+1).PrivateKey(p.key)))
			sts = append(sts, st)
		}
		for _, st := range sts {
			sim.StepUntil(Txn(st.TxID).Completes())
		}
	}

	// The consensus layer has been comparing all three shard counts on every
	// block; a divergence would already have failed a step. Verify the
	// traffic itself landed.
	for _, p := range parties {
		savings := GetAccount[*TokenAccount](t, sim.DatabaseFor(p.id), p.id.JoinPath("savings"))
		require.EqualValues(t, rounds, savings.Balance.Int64())
	}

	// And let the tail of synthetics and anchors settle under comparison too.
	sim.StepN(20)
}
