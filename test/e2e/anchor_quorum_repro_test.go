// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package e2e

import (
	"context"
	"gitlab.com/accumulatenetwork/accumulate/internal/api/private"
	api "gitlab.com/accumulatenetwork/accumulate/pkg/api/v3"
	"gitlab.com/accumulatenetwork/accumulate/pkg/errors"
	"testing"

	"github.com/stretchr/testify/require"
	"gitlab.com/accumulatenetwork/accumulate/internal/core"
	"gitlab.com/accumulatenetwork/accumulate/internal/database"
	"gitlab.com/accumulatenetwork/accumulate/pkg/build"
	"gitlab.com/accumulatenetwork/accumulate/pkg/url"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
	. "gitlab.com/accumulatenetwork/accumulate/protocol"
	. "gitlab.com/accumulatenetwork/accumulate/test/harness"
	. "gitlab.com/accumulatenetwork/accumulate/test/helpers"
	"gitlab.com/accumulatenetwork/accumulate/test/simulator"
	acctesting "gitlab.com/accumulatenetwork/accumulate/test/testing"
)

// Soak 20260905T225751Z: two BVNs by four validators, anchors collected below
// quorum and never executed once load began. Reproduce: the same topology,
// cross-partition load for many blocks, every anchor stream must keep
// delivering.
func TestAnchorsDeliverUnderLoadFourValidators(t *testing.T) {
	t.Run("memory", func(t *testing.T) { anchorsDeliverUnderLoad(t, nil) })
	t.Run("bcdb", func(t *testing.T) {
		anchorsDeliverUnderLoad(t, simulator.WithDatabase(simulator.BcdbDbOpener(t.TempDir(), func(err error) { require.NoError(t, err) })))
	})
}

func anchorsDeliverUnderLoad(t *testing.T, opener simulator.Option) {
	var timestamp uint64
	globals := new(core.GlobalValues)
	globals.ExecutorVersion = ExecutorVersionLatest
	opts := []simulator.Option{
		simulator.SimpleNetwork(t.Name(), 2, 4),
		simulator.GenesisWith(GenesisTime, globals),
	}
	if opener != nil {
		opts = append(opts, opener)
	}
	sim := NewSim(t, opts...)

	alice := acctesting.GenerateKey("Alice")
	aliceUrl := acctesting.AcmeLiteAddressStdPriv(alice)
	alicePart, err := sim.Router().RouteAccount(aliceUrl)
	require.NoError(t, err)
	var bobUrl *url.URL
	for i := 0; ; i++ {
		bob := acctesting.GenerateKey("Bob", i)
		bobUrl = acctesting.AcmeLiteAddressStdPriv(bob)
		bobPart, err := sim.Router().RouteAccount(bobUrl)
		require.NoError(t, err)
		if alicePart != bobPart {
			break
		}
	}
	MakeLiteTokenAccount(t, sim.DatabaseFor(aliceUrl), alice[32:], AcmeUrl())

	delivered := func(part string, source *url.URL) uint64 {
		var n uint64
		View(t, sim.Database(part), func(batch *database.Batch) {
			var ledger *protocol.AnchorLedger
			require.NoError(t, batch.Account(protocol.PartitionUrl(part).JoinPath(protocol.AnchorPool)).Main().GetAs(&ledger))
			n = ledger.Partition(source).Delivered
		})
		return n
	}

	// Load: a transfer every block for a while
	var st []*protocol.TransactionStatus
	for i := 0; i < 60; i++ {
		st = append(st, sim.SubmitTxnSuccessfully(MustBuild(t,
			build.Transaction().For(aliceUrl).
				SendTokens(1, 0).To(bobUrl).
				SignWith(aliceUrl).Version(1).Timestamp(&timestamp).PrivateKey(alice))))
		sim.StepN(1)
	}
	bobPart, _ := sim.Router().RouteAccount(bobUrl)
	dump := func() {
		View(t, sim.Database(alicePart), func(batch *database.Batch) {
			var ledger *protocol.SyntheticLedger
			require.NoError(t, batch.Account(protocol.PartitionUrl(alicePart).JoinPath(protocol.Synthetic)).Main().GetAs(&ledger))
			p := ledger.Partition(protocol.PartitionUrl(bobPart))
			t.Logf("%s -> %s: produced=%d (source ledger)", alicePart, bobPart, p.Produced)
		})
		View(t, sim.Database(bobPart), func(batch *database.Batch) {
			var ledger *protocol.SyntheticLedger
			require.NoError(t, batch.Account(protocol.PartitionUrl(bobPart).JoinPath(protocol.Synthetic)).Main().GetAs(&ledger))
			p := ledger.Partition(protocol.PartitionUrl(alicePart))
			t.Logf("%s -> %s: delivered=%d (destination ledger)", alicePart, bobPart, p.Delivered)
		})
		t.Logf("Directory delivered %s anchors: %d; %s delivered Directory anchors: %d", alicePart, delivered(Directory, protocol.PartitionUrl(alicePart)), alicePart, delivered(alicePart, protocol.DnUrl()))
		ranger, ok := sim.S.Services().Private().(private.SequenceRanger)
		require.True(t, ok)
		recs, err := ranger.SequenceRange(context.Background(), protocol.PartitionUrl(alicePart).JoinPath(protocol.Synthetic), protocol.PartitionUrl(bobPart), 1, 4096, private.SequenceOptions{})
		t.Logf("range answer from %s for %s: %d records, err=%v", alicePart, bobPart, len(recs), err)
		// The store's view of every transfer against the API's view
		var dbDelivered, apiOK, apiErr, apiOther int
		View(t, sim.Database(alicePart), func(batch *database.Batch) {
			for _, st := range st {
				h := st.TxID.Hash()
				v, err := batch.Transaction(h[:]).Status().Get()
				if err == nil && v.Delivered() {
					dbDelivered++
				}
			}
		})
		for _, st := range st {
			r, err := api.Querier2{Querier: sim.S.Services()}.QueryTransaction(context.Background(), st.TxID, nil)
			switch {
			case err != nil:
				apiErr++
				if apiErr == 1 {
					t.Logf("api error example: %v", err)
				}
			case r.Status == errors.Delivered:
				apiOK++
			default:
				apiOther++
				if apiOther == 1 {
					t.Logf("api status example: %v", r.Status)
				}
			}
		}
		t.Logf("%d transfers: store says delivered=%d; api says delivered=%d error=%d other=%d", len(st), dbDelivered, apiOK, apiErr, apiOther)
		for _, p := range sim.S.Partitions() {
			h := sim.S.Partition(p.ID).Heals()
			t.Logf("%s heals: synthetic=%d anchor=%d requests=%d misses=%d", p.ID, h.Synthetic.Load(), h.Anchor.Load(), h.Requests.Load(), h.Misses.Load())
		}
	}
	t.Cleanup(func() {
		if t.Failed() {
			dump()
		}
	})
	for _, st := range st {
		sim.StepUntilN(200, Txn(st.TxID).Succeeds(), Txn(st.TxID).Produced().Succeeds())
	}
	sim.StepN(20)

	dn := delivered(Directory, protocol.DnUrl())
	t.Logf("Directory delivered its own anchors: %d", dn)
	for _, bvn := range []string{"BVN0", "BVN1"} {
		t.Logf("%s delivered Directory anchors: %d; Directory delivered %s anchors: %d", bvn, delivered(bvn, protocol.DnUrl()), bvn, delivered(Directory, protocol.PartitionUrl(bvn)))
		require.Greater(t, delivered(bvn, protocol.DnUrl()), uint64(40), "%s stopped delivering Directory anchors", bvn)
		require.Greater(t, delivered(Directory, protocol.PartitionUrl(bvn)), uint64(40), "the Directory stopped delivering %s anchors", bvn)
	}
}
