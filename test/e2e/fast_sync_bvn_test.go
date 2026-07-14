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
	"gitlab.com/accumulatenetwork/accumulate/internal/node/config"
	"gitlab.com/accumulatenetwork/accumulate/pkg/build"
	"gitlab.com/accumulatenetwork/accumulate/pkg/errors"
	. "gitlab.com/accumulatenetwork/accumulate/protocol"
	. "gitlab.com/accumulatenetwork/accumulate/test/harness"
	. "gitlab.com/accumulatenetwork/accumulate/test/helpers"
	"gitlab.com/accumulatenetwork/accumulate/test/simulator"
	acctesting "gitlab.com/accumulatenetwork/accumulate/test/testing"
)

// TestFastSyncBVN syncs a BVN partition (#4058 phase 3b): the directory spine
// is the trust base, and the BVN's pinned state root is proven into it via
// the directory's record of the BVN's anchors — no BVN validator quorum is
// collected (those signatures live on the directory, not the BVN).
func TestFastSyncBVN(t *testing.T) {
	g := new(core.GlobalValues)
	g.Globals = new(NetworkGlobals)
	g.Globals.MajorBlockSchedule = "* * * * *" // Once a minute (60 minor blocks)
	g.Globals.OperatorAcceptThreshold.Set(1, 3)
	g.ExecutorVersion = ExecutorVersionLatest
	sim := NewSim(t,
		simulator.SimpleNetwork(t.Name(), 1, 3),
		simulator.GenesisWith(GenesisTime, g),
	)

	// The trust anchor is always the DIRECTORY globals — every partition's
	// genesis snapshot carries the same network definition
	genesis := loadDirectoryGlobals(t, sim)

	// Put real state on the BVN
	alice := AccountUrl("alice")
	aliceKey := acctesting.GenerateKey(alice)
	MakeIdentity(t, sim.DatabaseFor(alice), alice, aliceKey[32:])
	CreditCredits(t, sim.DatabaseFor(alice), alice.JoinPath("book", "1"), 1e9)
	MakeAccount(t, sim.DatabaseFor(alice), &DataAccount{Url: alice.JoinPath("data")})

	// Run through a major block so the spine has something to walk
	sim.StepUntilN(200, MajorBlock(1))

	bvn := PartitionUrl("BVN0")
	_, ok := sim.S.Services().Private().(fastsync.Client)
	require.True(t, ok, "private client does not serve the fast-sync surface")

	// A bogus state root must not be served a receipt
	ranger := sim.S.Services().Private().(private.PartitionRootRanger)
	_, err := ranger.PartitionRootRange(context.Background(), bvn, [32]byte{1, 2, 3}, private.SequenceOptions{})
	require.Error(t, err)
	require.Equal(t, errors.NotFound, errors.Code(err), "a root the directory never recorded must be not-found")

	// The poll hook stands in for a live network: step the simulator and
	// touch the BVN so it keeps producing (and anchoring) non-empty blocks
	var pollCount int
	var trafficN uint64
	poll := func(context.Context) error {
		pollCount++
		require.Less(t, pollCount, 500, "sync never converged")
		if pollCount%5 == 1 {
			trafficN++
			sim.BuildAndSubmitTxnSuccessfully(
				build.Transaction().For(alice, "data").
					Body(&WriteData{Entry: &DoubleHashDataEntry{Data: [][]byte{{byte(trafficN)}}}}).
					SignWith(alice, "book", "1").Version(1).Timestamp(trafficN).PrivateKey(aliceKey))
		}
		sim.Step()
		return nil
	}

	// The full sync: directory spine walk, BVN snapshot pin, root receipt,
	// restore, and proof
	syncDB := database.OpenInMemory(nil)
	res, err := fastsync.Sync(context.Background(), fastsync.Options{
		Client:    sim.S.Services().Private().(fastsync.Client),
		Genesis:   genesis,
		Partition: config.NetworkUrl{URL: bvn},
		Database:  syncDB,
		Poll:      poll,
	})
	require.NoError(t, err)
	require.NotZero(t, res.Epoch.Block)
	require.NotZero(t, res.Epoch.StateRoot)

	// The restored database must hold the BVN's accounts with live state
	require.NoError(t, syncDB.View(func(batch *database.Batch) error {
		var identity *ADI
		require.NoError(t, batch.Account(alice).Main().GetAs(&identity), "restored state must contain alice")
		var ledger *SystemLedger
		require.NoError(t, batch.Account(bvn.JoinPath(Ledger)).Main().GetAs(&ledger))
		require.Equal(t, res.Epoch.Block, ledger.Index, "restored ledger must be at the epoch block")
		return nil
	}))
}
