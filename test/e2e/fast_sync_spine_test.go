// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package e2e

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
	"gitlab.com/accumulatenetwork/accumulate/internal/api/private"
	"gitlab.com/accumulatenetwork/accumulate/internal/core"
	"gitlab.com/accumulatenetwork/accumulate/internal/database"
	"gitlab.com/accumulatenetwork/accumulate/internal/fastsync"
	"gitlab.com/accumulatenetwork/accumulate/internal/node/config"
	"gitlab.com/accumulatenetwork/accumulate/pkg/build"
	"gitlab.com/accumulatenetwork/accumulate/pkg/errors"
	"gitlab.com/accumulatenetwork/accumulate/pkg/types/messaging"
	"gitlab.com/accumulatenetwork/accumulate/pkg/url"
	. "gitlab.com/accumulatenetwork/accumulate/protocol"
	. "gitlab.com/accumulatenetwork/accumulate/test/harness"
	"gitlab.com/accumulatenetwork/accumulate/test/simulator"
	acctesting "gitlab.com/accumulatenetwork/accumulate/test/testing"
)

// TestFastSyncSpine walks the directory's major-block spine (#4058): it runs
// a network through several major blocks with a validator added along the
// way, fetches the spine via MajorHeaderRange, and verifies every major
// block's closing anchor against the validator set tracked by induction from
// the genesis state — including across the churn.
func TestFastSyncSpine(t *testing.T) {
	g := new(core.GlobalValues)
	g.Globals = new(NetworkGlobals)
	g.Globals.MajorBlockSchedule = "* * * * *" // Once a minute (60 minor blocks)
	g.Globals.OperatorAcceptThreshold.Set(1, 3)
	g.ExecutorVersion = ExecutorVersionLatest
	sim := NewSim(t,
		simulator.SimpleNetwork(t.Name(), 1, 3),
		simulator.GenesisWith(GenesisTime, g),
	)

	// Capture the trust-anchor state — in production this comes from the
	// pinned genesis snapshot
	genesis := loadDirectoryGlobals(t, sim)

	// Run through the first major block
	sim.StepUntilN(200, MajorBlock(1))

	// Churn: add a validator to the directory. The simulator's consensus
	// nodes are fixed at construction, so the new validator joins inactive
	// (a follower) — the network definition still changes and the change
	// still rides a directory anchor, which is what the induction must track.
	current := loadDirectoryGlobals(t, sim)
	newKey := acctesting.GenerateKey(t.Name(), "new-validator")
	current.Network.AddValidator(newKey[32:], Directory, false)
	current.Network.Version++
	st := sim.BuildAndSubmitTxnSuccessfully(
		build.Transaction().For(DnUrl(), Network).
			Body(&WriteData{Entry: current.FormatNetwork(), WriteToState: true}).
			SignWith(DnUrl(), Operators, "1").Version(1).Timestamp(1).Signer(sim.SignWithNode(Directory, 0)))
	sim.StepUntil(Txn(st.TxID).Completes())

	// Run through two more major blocks so the new set signs spine anchors
	sim.StepUntilN(200, MajorBlock(2))
	sim.StepUntilN(200, MajorBlock(3))

	// Give the anchor for the last major block's closing minor block time to
	// be produced and executed
	sim.StepN(10)

	// Fetch the spine
	ranger, ok := sim.S.Services().Private().(private.MajorHeaderRanger)
	require.True(t, ok, "private client does not serve major header ranges")
	records, err := ranger.MajorHeaderRange(context.Background(), DnUrl(), 1, 3, private.SequenceOptions{})
	require.NoError(t, err)
	require.Len(t, records, 3)

	// Walk it from the trust anchor
	spine, err := fastsync.NewSpine(genesis, 1)
	require.NoError(t, err)
	for _, r := range records {
		require.NoError(t, spine.Advance(r), "advance past major block %d", r.Index)
	}
	require.Equal(t, uint64(4), spine.NextMajor)
	require.NotZero(t, spine.LastMinorBlock)
	require.NotZero(t, spine.StateTreeAnchor)

	// The induction must have picked up the added validator
	require.True(t, hasValidator(spine.Globals().Network, newKey[32:]),
		"the walked validator set is missing the added validator")

	// A tampered anchor body must be rejected
	tampered := records[0].Copy()
	anchorBody(t, tampered).StateTreeAnchor[0]++
	spine2, err := fastsync.NewSpine(genesis, 1)
	require.NoError(t, err)
	require.Error(t, spine2.Advance(tampered), "a tampered anchor must not verify")

	// A sub-quorum record must be rejected
	short := records[0].Copy()
	short.Signatures = short.Signatures[:1]
	spine3, err := fastsync.NewSpine(genesis, 1)
	require.NoError(t, err)
	require.Error(t, spine3.Advance(short), "a sub-quorum anchor must not verify")

	// Records cannot be applied out of order
	spine4, err := fastsync.NewSpine(genesis, 1)
	require.NoError(t, err)
	require.Error(t, spine4.Advance(records[1]), "major block 2 must not apply before 1")

	// ——— Phase 2: bind the tail past the spine (#4058 MinorRootRange) ———

	// More churn in the tail, past the last major block
	current = loadDirectoryGlobals(t, sim)
	newKey2 := acctesting.GenerateKey(t.Name(), "tail-validator")
	current.Network.AddValidator(newKey2[32:], Directory, false)
	current.Network.Version++
	st = sim.BuildAndSubmitTxnSuccessfully(
		build.Transaction().For(DnUrl(), Network).
			Body(&WriteData{Entry: current.FormatNetwork(), WriteToState: true}).
			SignWith(DnUrl(), Operators, "1").Version(1).Timestamp(2).Signer(sim.SignWithNode(Directory, 0)))
	sim.StepUntil(Txn(st.TxID).Completes())
	sim.StepN(10)

	// Bind the epoch: chunk from the spine's position to the tip
	epoch, ok := sim.S.Services().Private().(private.MinorRootRanger)
	require.True(t, ok, "private client does not serve minor root ranges")
	before := spine.LastMinorBlock
	for {
		r, err := epoch.MinorRootRange(context.Background(), DnUrl(), spine.LastMinorBlock, 0, private.SequenceOptions{})
		require.NoError(t, err)
		require.NoError(t, spine.AdvanceEpoch(r), "advance epoch past block %d", spine.LastMinorBlock)
		if len(r.RootProof.Elements) < int(MaxReceiptListElements) {
			break // reached the tip
		}
	}
	require.Greater(t, spine.LastMinorBlock, before, "the epoch must advance past the spine")
	require.True(t, hasValidator(spine.Globals().Network, newKey2[32:]),
		"the walked validator set is missing the tail validator")

	// Rebuild a second walker at the spine position for the negative cases
	spineT, err := fastsync.NewSpine(genesis, 1)
	require.NoError(t, err)
	for _, r := range records {
		require.NoError(t, spineT.Advance(r))
	}

	// A tampered root proof must be rejected. Fetch a dedicated record —
	// Copy shares the element buffers, so tampering must not touch the
	// record used for the positive case.
	tampered2, err := epoch.MinorRootRange(context.Background(), DnUrl(), before, 0, private.SequenceOptions{})
	require.NoError(t, err)
	tampered2.RootProof.Elements[0][0]++
	require.Error(t, spineT.AdvanceEpoch(tampered2), "a tampered root proof must not verify")

	// The real record still verifies from the same position
	r2, err := epoch.MinorRootRange(context.Background(), DnUrl(), before, 0, private.SequenceOptions{})
	require.NoError(t, err)
	require.NoError(t, spineT.AdvanceEpoch(r2))

	// A replayed record must be rejected — it no longer advances
	require.Error(t, spineT.AdvanceEpoch(r2), "a replayed epoch record must not verify")

	// ——— Phase 3: fetch the epoch snapshot and restore it (#4058) ———

	snap, ok := sim.S.Services().Private().(private.SnapshotRanger)
	require.True(t, ok, "private client does not serve snapshot ranges")
	file, err := os.Create(filepath.Join(t.TempDir(), "state.snapshot"))
	require.NoError(t, err)
	defer file.Close()

	// Pinning succeeds only at a provable moment — when the current block
	// prepared an anchor that the next block will record with this state's
	// root. Submit a transaction to open the window and retry until it hits.
	current = loadDirectoryGlobals(t, sim)
	newKey3 := acctesting.GenerateKey(t.Name(), "epoch-validator")
	current.Network.AddValidator(newKey3[32:], Directory, false)
	current.Network.Version++
	sim.BuildAndSubmitTxnSuccessfully(
		build.Transaction().For(DnUrl(), Network).
			Body(&WriteData{Entry: current.FormatNetwork(), WriteToState: true}).
			SignWith(DnUrl(), Operators, "1").Version(1).Timestamp(3).Signer(sim.SignWithNode(Directory, 0)))

	var pinned fastsync.Epoch
	for i := 0; ; i++ {
		pinned, err = fastsync.FetchSnapshot(context.Background(), snap, DnUrl(), file)
		if err == nil {
			break
		}
		require.Equal(t, errors.NotReady, errors.Code(err), "pin must only fail as not-ready, got %v", err)
		require.Less(t, i, 100, "the pin window never opened")
		sim.Step()
	}
	require.GreaterOrEqual(t, pinned.Block, spine.LastMinorBlock)

	for i := 0; spine.LastMinorBlock < pinned.Block; i++ {
		r, err := epoch.MinorRootRange(context.Background(), DnUrl(), spine.LastMinorBlock, pinned.Block, private.SequenceOptions{})
		if err != nil {
			// Not found: the anchor is not produced yet. Not ready: it has
			// not reached quorum yet. Both resolve with more blocks.
			code := errors.Code(err)
			require.True(t, code == errors.NotReady || code == errors.NotFound,
				"binding must only fail as not-ready or not-found, got %v", err)
			require.Less(t, i, 100, "the epoch block's anchor never reached quorum")
			sim.Step()
			continue
		}
		require.NoError(t, spine.AdvanceEpoch(r))
	}
	require.Equal(t, pinned.Block, spine.LastMinorBlock, "the epoch block must be anchored exactly")

	// Restore into a fresh database. RestoreSnapshot rebuilds the BPT from
	// the restored accounts and requires its root to equal the verified
	// StateTreeAnchor — the complete-set proof of every account state.
	restored := database.OpenInMemory(nil)
	require.NoError(t, fastsync.RestoreSnapshot(restored, file, config.NetworkUrl{URL: DnUrl()}, spine.StateTreeAnchor))

	// A restored database with any account tampered must not verify
	tamperedDB := database.OpenInMemory(nil)
	require.NoError(t, tamperedDB.Update(func(batch *database.Batch) error {
		return batch.Account(DnUrl().JoinPath("tampered")).Main().Put(&UnknownAccount{Url: DnUrl().JoinPath("tampered")})
	}))
	err = fastsync.RestoreSnapshot(tamperedDB, file, config.NetworkUrl{URL: DnUrl()}, spine.StateTreeAnchor)
	require.Error(t, err, "a database with extra state must not match the verified root")

	// ——— The full Sync orchestrator, from genesis to restored state ———

	// The poll hook stands in for a live network: it steps the simulator
	// and generates traffic so anchors keep being prepared
	var pollCount int
	trafficTimestamp := uint64(4)
	poll := func(context.Context) error {
		pollCount++
		require.Less(t, pollCount, 500, "sync never converged")
		if pollCount%5 == 1 {
			g := loadDirectoryGlobals(t, sim)
			k := acctesting.GenerateKey(t.Name(), "traffic", trafficTimestamp)
			g.Network.AddValidator(k[32:], Directory, false)
			g.Network.Version++
			sim.BuildAndSubmitTxnSuccessfully(
				build.Transaction().For(DnUrl(), Network).
					Body(&WriteData{Entry: g.FormatNetwork(), WriteToState: true}).
					SignWith(DnUrl(), Operators, "1").Version(1).Timestamp(trafficTimestamp).Signer(sim.SignWithNode(Directory, 0)))
			trafficTimestamp++
		}
		sim.Step()
		return nil
	}

	syncDB := database.OpenInMemory(nil)
	res, err := fastsync.Sync(context.Background(), fastsync.Options{
		Client:    sim.S.Services().Private().(fastsync.Client),
		Genesis:   genesis,
		Partition: config.NetworkUrl{URL: DnUrl()},
		Database:  syncDB,
		Poll:      poll,
	})
	require.NoError(t, err)
	require.NotZero(t, res.Epoch.Block)
	require.Equal(t, res.Epoch.Block, res.Spine.LastMinorBlock)
	require.GreaterOrEqual(t, res.Spine.NextMajor, uint64(4), "the sync must have walked the whole spine")
}

func loadDirectoryGlobals(t *testing.T, sim *Sim) *core.GlobalValues {
	t.Helper()
	g := new(core.GlobalValues)
	require.NoError(t, sim.Database(Directory).View(func(batch *database.Batch) error {
		return g.Load(DnUrl(), func(account *url.URL, target interface{}) error {
			return batch.Account(account).Main().GetAs(target)
		})
	}))
	return g
}

func hasValidator(def *NetworkDefinition, key []byte) bool {
	for _, v := range def.Validators {
		if string(v.PublicKey) == string(key) {
			return true
		}
	}
	return false
}

func anchorBody(t *testing.T, r *private.MajorHeaderRecord) *DirectoryAnchor {
	t.Helper()
	txn, ok := r.Anchor.Message.(*messaging.TransactionMessage)
	require.True(t, ok)
	body, ok := txn.Transaction.Body.(*DirectoryAnchor)
	require.True(t, ok)
	return body
}
