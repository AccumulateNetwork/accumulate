// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package e2e

import (
	"bytes"
	"context"
	"testing"

	"github.com/stretchr/testify/require"
	"gitlab.com/accumulatenetwork/accumulate/internal/api/private"
	"gitlab.com/accumulatenetwork/accumulate/internal/core"
	"gitlab.com/accumulatenetwork/accumulate/internal/database"
	"gitlab.com/accumulatenetwork/accumulate/pkg/build"
	"gitlab.com/accumulatenetwork/accumulate/pkg/types/messaging"
	"gitlab.com/accumulatenetwork/accumulate/pkg/url"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
	. "gitlab.com/accumulatenetwork/accumulate/protocol"
	. "gitlab.com/accumulatenetwork/accumulate/test/harness"
	. "gitlab.com/accumulatenetwork/accumulate/test/helpers"
	"gitlab.com/accumulatenetwork/accumulate/test/simulator"
	acctesting "gitlab.com/accumulatenetwork/accumulate/test/testing"
)

// A source keeps one synthetic chain per destination (executor spec, "One
// chain per pair, one stage per chain"): a block that sends to two
// destinations appends to two chains, each chain holds only that
// destination's entries in sequence order with entry n-1 being sequence
// number n, and a collection proof for a destination covers its chain alone,
// so the proof's elements are exactly the destination's entries and its
// starting index is the sequence number less one.
func TestSyntheticChainPerDestination(t *testing.T) {
	var timestamp uint64
	const perDestination = 3

	globals := new(core.GlobalValues)
	globals.ExecutorVersion = ExecutorVersionLatest
	sim := NewSim(t,
		simulator.SimpleNetwork(t.Name(), 3, 1),
		simulator.GenesisWith(GenesisTime, globals),
	)

	alice := acctesting.GenerateKey("Alice")
	aliceUrl := acctesting.AcmeLiteAddressStdPriv(alice)
	alicePart, err := sim.Router().RouteAccount(aliceUrl)
	require.NoError(t, err)

	// Two recipients on two partitions, neither alice's
	recipients := map[string]*url.URL{}
	for i := 0; len(recipients) < 2; i++ {
		key := acctesting.GenerateKey("Recipient", i)
		u := acctesting.AcmeLiteAddressStdPriv(key)
		part, err := sim.Router().RouteAccount(u)
		require.NoError(t, err)
		if part != alicePart {
			recipients[part] = u
		}
	}
	MakeLiteTokenAccount(t, sim.DatabaseFor(aliceUrl), alice[32:], AcmeUrl())

	// Send to both destinations in the same blocks
	var st []*protocol.TransactionStatus
	for i := 0; i < perDestination; i++ {
		for _, to := range recipients {
			st = append(st, sim.SubmitTxnSuccessfully(MustBuild(t,
				build.Transaction().For(aliceUrl).
					SendTokens(1, protocol.AcmePrecisionPower).To(to).
					SignWith(aliceUrl).Version(1).Timestamp(&timestamp).PrivateKey(alice))))
		}
		sim.StepN(1)
	}
	for _, st := range st {
		sim.StepUntilN(200, Txn(st.TxID).Succeeds(), Txn(st.TxID).Produced().Succeeds())
	}
	// Past the in-flight window, so the sequencer serves the range from the
	// cache
	sim.StepN(20)

	// Each destination's chain holds exactly its entries, in sequence order
	chains := map[string][][]byte{}
	View(t, sim.DatabaseFor(aliceUrl), func(batch *database.Batch) {
		ledger := batch.Account(protocol.PartitionUrl(alicePart).JoinPath(protocol.Synthetic))
		for part := range recipients {
			chain, err := ledger.SyntheticChain(part).Get()
			require.NoError(t, err)
			require.GreaterOrEqual(t, chain.Height(), int64(perDestination), "chain to %s", part)
			for i := int64(0); i < chain.Height(); i++ {
				hash, err := chain.Entry(i)
				require.NoError(t, err)
				var seq *messaging.SequencedMessage
				require.NoError(t, batch.Message2(hash).Main().GetAs(&seq))
				require.Equal(t, protocol.PartitionUrl(part).String(), seq.Destination.String(), "entry %d of the chain to %s", i, part)
				require.Equal(t, uint64(i+1), seq.Number, "entry %d of the chain to %s is sequence number %d", i, part, i+1)
				chains[part] = append(chains[part], hash)
			}
		}
	})

	// A collection proof for a destination covers its chain alone
	ranger, ok := sim.S.Services().Private().(private.SequenceRanger)
	require.True(t, ok, "private client does not serve sequence ranges")
	for part, entries := range chains {
		records, err := ranger.SequenceRange(context.Background(),
			protocol.PartitionUrl(alicePart).JoinPath(protocol.Synthetic),
			protocol.PartitionUrl(part), 1, uint64(len(entries)), private.SequenceOptions{})
		require.NoError(t, err, "range for %s", part)
		require.Len(t, records, len(entries))
		list := records[len(records)-1].SourceReceiptList
		require.NotNil(t, list, "the range must carry a collection proof")
		require.True(t, list.Validate(nil), "the collection proof must be valid")
		require.Zero(t, list.MerkleState.Count, "the proof starts at sequence number 1, index 0")
		require.Len(t, list.Elements, len(entries), "the proof covers the destination's entries and nothing else")
		for i, e := range list.Elements {
			require.True(t, bytes.Equal(e, entries[i]), "element %d of the proof for %s is the chain's entry %d", i, part, i)
		}
	}
}
