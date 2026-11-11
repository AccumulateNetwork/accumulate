// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package e2e

import (
	"testing"

	"github.com/stretchr/testify/require"
	"gitlab.com/accumulatenetwork/accumulate/internal/database"
	"gitlab.com/accumulatenetwork/accumulate/pkg/build"
	"gitlab.com/accumulatenetwork/accumulate/pkg/consensus/types"
	"gitlab.com/accumulatenetwork/accumulate/pkg/url"
	. "gitlab.com/accumulatenetwork/accumulate/protocol"
	. "gitlab.com/accumulatenetwork/accumulate/test/harness"
	"gitlab.com/accumulatenetwork/accumulate/test/simulator"
	acctesting "gitlab.com/accumulatenetwork/accumulate/test/testing"
)

// The block that activates a new network definition records in each
// partition's system ledger the DAG round its validators take over at -- the
// block's leader round plus the activation margin -- and the committee before
// it: its version, its validators on the partition and the round it took over
// at. A node that restarts or joins reads them back from its state (#4593).
// The simulator commits no DAG-BFT leader, so the round is the margin itself.
func TestTheLedgerRecordsTheRoundANetworkDefinitionTakesOverAt(t *testing.T) {
	sim := newDefinitionSim(t, ExecutorVersionV2Canaveral)
	genesis := changeTheNetworkDefinition(t, sim, "first", 1)
	for _, part := range []string{Directory, "BVN0"} {
		l := systemLedger(t, sim, part)
		require.Equal(t, uint64(types.CommitteeActivationMargin), l.CommitteeActivationRound, part)
		require.Equal(t, genesis.Version, l.PreviousNetworkVersion, part)
		require.Equal(t, activeKeys(genesis, part), l.PreviousValidators, part)
		require.Zero(t, l.PreviousCommitteeActivationRound, "%s: the genesis committee", part)
	}

	// A second change carries the first change's committee and round.
	first := changeTheNetworkDefinition(t, sim, "second", 2)
	for _, part := range []string{Directory, "BVN0"} {
		l := systemLedger(t, sim, part)
		require.Equal(t, first.Version, l.PreviousNetworkVersion, part)
		require.Equal(t, activeKeys(first, part), l.PreviousValidators, part)
		require.Equal(t, uint64(types.CommitteeActivationMargin), l.PreviousCommitteeActivationRound, part)
	}
}

// Every network-definition write in a block sees the same active
// definition, so the ledger shifts the previous committee's round only on the
// block's first: a second must not record as the previous round the one the
// first just set, or a joiner between the two committees could order nothing.
func TestTwoNetworkDefinitionWritesInABlockKeepThePreviousRound(t *testing.T) {
	sim := newDefinitionSim(t, ExecutorVersionV2Canaveral)
	require.NoError(t, sim.Database(Directory).Update(func(batch *database.Batch) error {
		record := batch.Account(DnUrl().JoinPath(Ledger)).Main()
		var l *SystemLedger
		if err := record.GetAs(&l); err != nil {
			return err
		}
		l.CommitteeActivationRound = 100
		return record.Put(l)
	}))

	def := loadDirectoryGlobals(t, sim)
	var ids []*url.TxID
	for i, seed := range []string{"first", "second"} {
		newKey := acctesting.GenerateKey(t.Name(), seed)
		def.Network.AddValidator(newKey[32:], "BVN0", false)
		def.Network.Version++
		st := sim.BuildAndSubmitTxnSuccessfully(build.Transaction().For(DnUrl(), Network).
			Body(&WriteData{Entry: def.FormatNetwork(), WriteToState: true}).
			SignWith(DnUrl(), Operators, "1").Version(1).Timestamp(uint64(i + 1)).Signer(sim.SignWithNode(Directory, 0)))
		ids = append(ids, st.TxID)
	}
	for _, id := range ids {
		sim.StepUntil(Txn(id).Completes())
	}
	sim.StepN(20)

	// Had the writes landed in two blocks, the second would rightly have
	// recorded the margin as the previous round.
	l := systemLedger(t, sim, Directory)
	require.Equal(t, uint64(types.CommitteeActivationMargin), l.CommitteeActivationRound)
	require.Equal(t, uint64(100), l.PreviousCommitteeActivationRound)
}

// Below v2-canaveral the ledger records none of the fields: Kermit runs
// v2-kourou, and what a block writes changes only at an activation.
func TestTheLedgerRecordsNoActivationRoundBeforeCanaveral(t *testing.T) {
	sim := newDefinitionSim(t, ExecutorVersionV2Kourou)
	changeTheNetworkDefinition(t, sim, "first", 1)
	for _, part := range []string{Directory, "BVN0"} {
		l := systemLedger(t, sim, part)
		require.Zero(t, l.CommitteeActivationRound, part)
		require.Zero(t, l.PreviousNetworkVersion, part)
		require.Empty(t, l.PreviousValidators, part)
		require.Zero(t, l.PreviousCommitteeActivationRound, part)
	}
}

func newDefinitionSim(t *testing.T, version ExecutorVersion) *Sim {
	t.Helper()
	sim := NewSim(t,
		simulator.SimpleNetwork(t.Name(), 1, 1),
		simulator.GenesisWithVersion(GenesisTime, version),
	)
	for _, part := range []string{Directory, "BVN0"} {
		require.Zero(t, systemLedger(t, sim, part).CommitteeActivationRound, "%s before any change", part)
	}
	return sim
}

func systemLedger(t *testing.T, sim *Sim, partition string) *SystemLedger {
	t.Helper()
	var l *SystemLedger
	require.NoError(t, sim.Database(partition).View(func(batch *database.Batch) error {
		return batch.Account(PartitionUrl(partition).JoinPath(Ledger)).Main().GetAs(&l)
	}))
	return l
}

// activeKeys is the keys of a definition's validators active on a partition.
func activeKeys(def *NetworkDefinition, partition string) [][]byte {
	var keys [][]byte
	for _, v := range def.Validators {
		if v.IsActiveOn(partition) {
			keys = append(keys, v.PublicKey)
		}
	}
	return keys
}

// changeTheNetworkDefinition adds an inactive validator, named by seed, to
// BVN0 and returns the network definition before the change. An active one
// would stall the simulator, which runs no node for it.
func changeTheNetworkDefinition(t *testing.T, sim *Sim, seed string, timestamp uint64) *NetworkDefinition {
	t.Helper()
	current := loadDirectoryGlobals(t, sim)
	before := current.Network.Copy()
	newKey := acctesting.GenerateKey(t.Name(), seed)
	current.Network.AddValidator(newKey[32:], "BVN0", false)
	current.Network.Version++
	st := sim.BuildAndSubmitTxnSuccessfully(build.Transaction().For(DnUrl(), Network).
		Body(&WriteData{Entry: current.FormatNetwork(), WriteToState: true}).
		SignWith(DnUrl(), Operators, "1").Version(1).Timestamp(timestamp).Signer(sim.SignWithNode(Directory, 0)))
	sim.StepUntil(Txn(st.TxID).Completes())
	sim.StepN(20)
	return before
}
