// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package e2e

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
	"gitlab.com/accumulatenetwork/accumulate/internal/api/private"
	"gitlab.com/accumulatenetwork/accumulate/internal/core"
	"gitlab.com/accumulatenetwork/accumulate/internal/database"
	"gitlab.com/accumulatenetwork/accumulate/internal/fastsync"
	"gitlab.com/accumulatenetwork/accumulate/pkg/build"
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
