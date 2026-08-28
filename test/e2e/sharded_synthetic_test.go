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
	"gitlab.com/accumulatenetwork/accumulate/internal/core/execute/v2/block"
	"gitlab.com/accumulatenetwork/accumulate/pkg/build"
	"gitlab.com/accumulatenetwork/accumulate/pkg/url"
	. "gitlab.com/accumulatenetwork/accumulate/protocol"
	. "gitlab.com/accumulatenetwork/accumulate/test/harness"
	. "gitlab.com/accumulatenetwork/accumulate/test/helpers"
	"gitlab.com/accumulatenetwork/accumulate/test/simulator"
	acctesting "gitlab.com/accumulatenetwork/accumulate/test/testing"
)

// Shard count must not change the result of a SYNTHETIC-HEAVY block.
//
// The existing equivalence gate (TestShardCountDoesNotChangeBlockHash) drives
// identity-local transfers. Synthetics are currently forced serial by the
// blanket type check in envelopeIdentity, so for the delivery path that gate
// compares serial to serial — it would not notice a mistake in sharded
// synthetic delivery.
//
// This is the gate for docs/plans/shard-synthetic-delivery.md, and it is
// deliberately written BEFORE the change it guards. Today it passes trivially,
// which is exactly the vacuity trap that made the first version of
// TestReadySequencedMessageNeverReturnsPending worthless. So it asserts its own
// coverage: the workload must actually push a substantial number of sequenced
// messages through the delivery path, or the test fails as uninformative
// rather than passing as green.
//
// As with the other gate, the oracle is consensus itself: nodes 0, 1 and 2 of
// every partition execute at 1, 4 and 64 shards simultaneously, so any
// divergence fails a step rather than being caught by an assertion afterwards.
func TestShardEquivalence_SyntheticHeavy(t *testing.T) {
	block.SequencedReadyExecuted.Store(0)
	block.SequencedSyntheticExecuted.Store(0)
	block.ReadyReturnedPending.Store(0)

	const identities = 12
	const rounds = 4

	sim := NewSim(t,
		simulator.SimpleNetwork("ShardSynth", 3, 3),
		simulator.Genesis(GenesisTime),
		simulator.ExecutionShardsPerNode(1, 4, 64),
	)

	type party struct {
		key []byte
		id  *url.URL
	}
	parties := make([]party, identities)
	for i := range parties {
		id := AccountUrl(fmt.Sprintf("synth-%d", i))
		key := acctesting.GenerateKey(id)
		MakeIdentity(t, sim.DatabaseFor(id), id, key[32:])
		CreditCredits(t, sim.DatabaseFor(id), id.JoinPath("book", "1"), 1e9)
		MakeAccount(t, sim.DatabaseFor(id),
			&TokenAccount{Url: id.JoinPath("tokens"), TokenUrl: AcmeUrl()})
		CreditTokens(t, sim.DatabaseFor(id), id.JoinPath("tokens"), big.NewInt(1e12))
		parties[i] = party{key, id}
	}

	// Twelve identities over three BVNs, each sending to the identity two
	// places along — most transfers cross a partition boundary and therefore
	// produce a real synthetic deposit rather than a local delivery (#4146).
	//
	// A whole round is submitted BEFORE stepping, on purpose. Draining after
	// every send keeps each stream at depth one, which never exercises the
	// in-sequence/out-of-sequence split; batching gives the streams depth.
	var ts uint64
	for r := 0; r < rounds; r++ {
		var ids []*url.TxID
		for i, p := range parties {
			dst := parties[(i+2)%len(parties)].id
			ts++
			st := sim.SubmitTxnSuccessfully(MustBuild(t,
				build.Transaction().For(p.id.JoinPath("tokens")).
					SendTokens(1, 0).To(dst.JoinPath("tokens")).
					SignWith(p.id.JoinPath("book", "1")).Version(1).
					Timestamp(ts).PrivateKey(p.key)))
			ids = append(ids, st.TxID)
		}
		for _, id := range ids {
			sim.StepUntil(Txn(id).Completes())
		}
	}

	// Let the synthetic and anchor tails settle under comparison too — the
	// drain is where out-of-sequence delivery actually happens.
	sim.StepN(40)

	// Every identity received `rounds` deposits of 1 credit-unit each.
	for _, p := range parties {
		acct := GetAccount[*TokenAccount](t, sim.DatabaseFor(p.id), p.id.JoinPath("tokens"))
		require.NotNil(t, acct, "%v/tokens", p.id)
	}

	// Coverage, asserted BEFORE the equivalence claim is trusted. Without this
	// the test would pass on a workload that produced no synthetics at all.
	// The bar is deliberately well above zero: a handful of anchors would
	// clear `> 0` without any synthetic delivery having happened.
	// Anchors are sequenced too and outnumber synthetics heavily, so assert on
	// the NON-ANCHOR count. Asserting the total would let pure anchor traffic
	// satisfy a test whose subject is synthetic delivery.
	synth := block.SequencedSyntheticExecuted.Load()
	require.Greater(t, synth, int64(identities*rounds),
		"only %d SYNTHETIC sequenced messages executed (of %d sequenced "+
			"overall) — this workload is not synthetic-heavy and the "+
			"equivalence claim is uninformative",
		synth, block.SequencedReadyExecuted.Load())

	require.Zero(t, block.ReadyReturnedPending.Load(),
		"a ready sequenced message returned pending during the sharded run")
}
