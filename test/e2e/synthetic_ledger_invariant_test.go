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

// A READY sequenced message must never come back pending.
//
// This is the load-bearing assumption of sharded synthetic delivery
// (docs/plans/shard-synthetic-delivery.md). The ledger owner has to decide
// pending-vs-delivered BEFORE handing the transaction to its destination
// shard, which is only possible if the answer is `!ready` — derivable from the
// ledger alone — rather than a property of the execution result.
//
// It holds because the anchor proof is verified in SyntheticMessage.process
// BEFORE the sequenced executor runs: an unanchored message returns
// errors.Pending there and never reaches the ledger. By that point a message
// is either in-sequence and executable, or out of sequence.
//
// The first probe of this appeared to refute it — 376 "pending after
// execution" hits — but it was placed after BOTH branches of
// `if ready {execute} else {recordPending}`, so it counted out-of-sequence
// messages that never ran. With the branch recorded, all 376 were ready=false.
// This test exists so that mistake cannot be made twice: it drives real
// cross-partition traffic and asserts the counter stays at zero.
func TestReadySequencedMessageNeverReturnsPending(t *testing.T) {
	block.ReadyReturnedPending.Store(0)
	block.SequencedReadyExecuted.Store(0)

	const senders = 8
	const rounds = 3

	sim := NewSim(t,
		simulator.SimpleNetwork(t.Name(), 3, 3),
		simulator.Genesis(GenesisTime),
	)

	// Senders and receivers deliberately spread across partitions, so most
	// transfers produce a real cross-partition synthetic rather than a local
	// delivery (#4146), which is the path that touches the sequence ledger.
	type party struct {
		key []byte
		id  *url.URL
	}
	parties := make([]party, senders)
	for i := range parties {
		id := AccountUrl(fmt.Sprintf("inv-%d", i))
		key := acctesting.GenerateKey(id)
		MakeIdentity(t, sim.DatabaseFor(id), id, key[32:])
		CreditCredits(t, sim.DatabaseFor(id), id.JoinPath("book", "1"), 1e9)
		MakeAccount(t, sim.DatabaseFor(id),
			&TokenAccount{Url: id.JoinPath("tokens"), TokenUrl: AcmeUrl()})
		CreditTokens(t, sim.DatabaseFor(id), id.JoinPath("tokens"), big.NewInt(1e12))
		parties[i] = party{key, id}
	}

	// Each round every identity sends to the NEXT one, so every partition both
	// produces and receives, and the streams carry several messages each —
	// which is what exercises the in-sequence/out-of-sequence split.
	for r := 0; r < rounds; r++ {
		for i, p := range parties {
			dst := parties[(i+1)%len(parties)].id
			st := sim.BuildAndSubmitTxnSuccessfully(
				build.Transaction().
					For(p.id, "tokens").
					SendTokens(1, 0).To(dst, "tokens").
					SignWith(p.id, "book", "1").Version(1).
					Timestamp(uint64(r + 1)).PrivateKey(p.key))
			sim.StepUntil(Txn(st.TxID).Completes())
		}
	}

	// Let the synthetic and anchor tails settle — the out-of-sequence branch
	// is most likely during a drain, so the quiet period matters.
	sim.StepN(30)

	// Prove the path was exercised BEFORE asserting nothing went wrong on it.
	// The first version of this test drained after every send, so no sequenced
	// message was ever pending and the assertion below passed without testing
	// anything — inverting the invariant to fire on a case that happens 376
	// times in the full suite still left this test green.
	require.NotZero(t, block.SequencedReadyExecuted.Load(),
		"no sequenced message executed — this test is not exercising the "+
			"delivery path and its invariant assertion is vacuous")

	require.Zero(t, block.ReadyReturnedPending.Load(),
		"a ready sequenced message returned pending — sharded synthetic "+
			"delivery would advance the watermark on a transaction that did "+
			"not deliver (see docs/plans/shard-synthetic-delivery.md)")
}
