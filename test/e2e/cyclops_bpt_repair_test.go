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
	"gitlab.com/accumulatenetwork/accumulate/pkg/types/record"
	"gitlab.com/accumulatenetwork/accumulate/pkg/url"
	. "gitlab.com/accumulatenetwork/accumulate/protocol"
	. "gitlab.com/accumulatenetwork/accumulate/test/harness"
	. "gitlab.com/accumulatenetwork/accumulate/test/helpers"
	"gitlab.com/accumulatenetwork/accumulate/test/simulator"
	acctesting "gitlab.com/accumulatenetwork/accumulate/test/testing"
)

const plantedFakeByte = byte(0xC0)

// plantedFake is the corrupted BPT leaf value we use to simulate the
// post-reorg state drift on a target account.
var plantedFake = func() [32]byte {
	var b [32]byte
	for i := range b {
		b[i] = plantedFakeByte
	}
	return b
}()

// makeCyclopsSim builds a simulator network with a single BVN named
// the given partition ID, genesis at the given executor version, and
// returns the sim. Use partitionId="Cyclops" to engage the repair
// table; anything else exercises the no-op path.
func makeCyclopsSim(t *testing.T, partitionId string, version ExecutorVersion) *Sim {
	t.Helper()
	net := simulator.NewSimpleNetwork(t.Name(), 1, 1)
	net.Bvns[0].Id = partitionId
	return NewSim(t,
		simulator.WithNetwork(net),
		simulator.GenesisWithVersion(GenesisTime, version),
	)
}

// plantAdiWithCorruptLeaf creates an ADI and overwrites its BPT leaf
// with a deliberately-wrong value. Returns the ADI URL.
func plantAdiWithCorruptLeaf(t *testing.T, sim *Sim, adi *url.URL) {
	t.Helper()
	bvnDb := sim.DatabaseFor(adi)
	MakeIdentity(t, bvnDb, adi, acctesting.GenerateKey(adi)[32:])
	CreditCredits(t, bvnDb, adi.JoinPath("book", "1"), 1e9)

	Update(t, bvnDb, func(batch *database.Batch) {
		key := record.NewKey("Account", adi)
		require.NoError(t, batch.BPT().Insert(key, plantedFake[:]))
	})

	// Sanity: the planted corruption is in place.
	View(t, bvnDb, func(batch *database.Batch) {
		stored, err := batch.BPT().Get(record.NewKey("Account", adi))
		require.NoError(t, err)
		require.Equal(t, plantedFake[:], stored, "planted corruption must be present")
	})
}

// activateCyclopsBptRepair submits the protocol-version-activation
// transaction and steps until it commits + the version propagates.
func activateCyclopsBptRepair(t *testing.T, sim *Sim) {
	t.Helper()
	st := sim.BuildAndSubmitTxnSuccessfully(
		build.Transaction().For(DnUrl()).
			ActivateProtocolVersion(ExecutorVersionV2CyclopsBptRepair).
			SignWith(DnUrl(), Operators, "1").Version(1).Timestamp(1).
			Signer(sim.SignWithNode(Directory, 0)))
	sim.StepUntil(Txn(st.TxID).Succeeds())
	sim.StepN(20)
}

// readLeafAndHash returns the stored BPT leaf and recomputed
// account.Hash() for an account on the BVN.
func readLeafAndHash(t *testing.T, sim *Sim, u *url.URL) (stored, computed [32]byte) {
	t.Helper()
	View(t, sim.DatabaseFor(u), func(batch *database.Batch) {
		raw, err := batch.BPT().Get(record.NewKey("Account", u))
		require.NoError(t, err)
		require.Lenf(t, raw, 32, "BPT leaf for %v has wrong length", u)
		copy(stored[:], raw)
		h, err := batch.Account(u).Hash()
		require.NoError(t, err)
		computed = h
	})
	return
}

// TestCyclopsBptRepairActivation — the headline test. Activate the
// new version on a BVN named "Cyclops", verify a planted corruption
// on a Class-A target ADI is repaired (stored leaf now matches
// recomputed account.Hash()).
func TestCyclopsBptRepairActivation(t *testing.T) {
	sim := makeCyclopsSim(t, "Cyclops", ExecutorVersionV2Jiuquan)
	adi := AccountUrl("csrc.acme")
	plantAdiWithCorruptLeaf(t, sim, adi)

	activateCyclopsBptRepair(t, sim)

	stored, computed := readLeafAndHash(t, sim, adi)
	require.NotEqual(t, plantedFake, stored,
		"stored leaf should no longer be the planted fake")
	require.Equal(t, computed, stored,
		"stored leaf should equal recomputed account.Hash() after repair")
}

// TestCyclopsBptRepair_PreActivationNoop — at version V2Jiuquan
// (less than V2CyclopsBptRepair), the planted corruption stays
// planted. Confirms version gating.
func TestCyclopsBptRepair_PreActivationNoop(t *testing.T) {
	sim := makeCyclopsSim(t, "Cyclops", ExecutorVersionV2Jiuquan)
	adi := AccountUrl("csrc.acme")
	plantAdiWithCorruptLeaf(t, sim, adi)

	// Step blocks WITHOUT activating the new version.
	sim.StepN(40)

	// Planted corruption should still be present.
	View(t, sim.DatabaseFor(adi), func(batch *database.Batch) {
		stored, err := batch.BPT().Get(record.NewKey("Account", adi))
		require.NoError(t, err)
		require.Equal(t, plantedFake[:], stored,
			"without activation, planted corruption must persist")
	})
}

// TestCyclopsBptRepair_NonCyclopsNoop — activation on a sim BVN
// named "BVN0" (not "Cyclops") is a true no-op: planted corruption
// stays untouched, no panic. This is the case for the production DN
// partition and for any other BVN that may exist alongside Cyclops.
func TestCyclopsBptRepair_NonCyclopsNoop(t *testing.T) {
	sim := makeCyclopsSim(t, "BVN0", ExecutorVersionV2Jiuquan)
	adi := AccountUrl("csrc.acme")
	plantAdiWithCorruptLeaf(t, sim, adi)

	activateCyclopsBptRepair(t, sim)

	// Non-Cyclops partition has no targets in the table; the planted
	// corruption must survive activation.
	View(t, sim.DatabaseFor(adi), func(batch *database.Batch) {
		stored, err := batch.BPT().Get(record.NewKey("Account", adi))
		require.NoError(t, err)
		require.Equal(t, plantedFake[:], stored,
			"on a non-Cyclops partition, activation must not alter the planted leaf")
	})
}

// TestCyclopsBptRepair_Idempotent — the activation runs once when
// the version transitions, and never again. Stepping additional
// blocks after activation should not re-trigger the repair logic.
// We verify this by capturing the post-repair leaf and the BPT root,
// then stepping more blocks and confirming both are unchanged.
func TestCyclopsBptRepair_Idempotent(t *testing.T) {
	sim := makeCyclopsSim(t, "Cyclops", ExecutorVersionV2Jiuquan)
	adi := AccountUrl("csrc.acme")
	plantAdiWithCorruptLeaf(t, sim, adi)

	activateCyclopsBptRepair(t, sim)

	// Capture leaf + BPT root just after activation.
	postLeaf, _ := readLeafAndHash(t, sim, adi)
	var postRoot [32]byte
	View(t, sim.DatabaseFor(adi), func(batch *database.Batch) {
		r, err := batch.GetBptRootHash()
		require.NoError(t, err)
		postRoot = r
	})

	// Step many additional blocks — repair must not re-fire.
	sim.StepN(50)

	postLeaf2, _ := readLeafAndHash(t, sim, adi)
	var postRoot2 [32]byte
	View(t, sim.DatabaseFor(adi), func(batch *database.Batch) {
		r, err := batch.GetBptRootHash()
		require.NoError(t, err)
		postRoot2 = r
	})

	require.Equal(t, postLeaf, postLeaf2,
		"target leaf must be stable after activation; repair must not re-fire")
	require.Equal(t, postRoot, postRoot2,
		"BPT root must be stable after activation block (no transactions submitted)")
}

// TestCyclopsBptRepair_OrphanClass — Class B target (orphan, no
// chains in repair entry). The repair just MarkDirty's the account
// so UpdateBPT writes the empty-state hash matching the body-less
// account's recomputed hash.
//
// We use kmutt.acme — the only orphan ADI in the table. We don't
// pre-create the body (kmutt.acme is body-less by design); we just
// plant a corrupt non-empty leaf in the BPT. After activation, the
// leaf should equal account.Hash() over the body-less state, which
// is the protocol's empty-state hash.
func TestCyclopsBptRepair_OrphanClass(t *testing.T) {
	sim := makeCyclopsSim(t, "Cyclops", ExecutorVersionV2Jiuquan)
	orphan := AccountUrl("kmutt.acme")

	// Plant a non-empty BPT leaf for an account with no body.
	bvnDb := sim.DatabaseFor(orphan)
	Update(t, bvnDb, func(batch *database.Batch) {
		require.NoError(t, batch.BPT().Insert(record.NewKey("Account", orphan), plantedFake[:]))
	})

	// Confirm the planted leaf is in place and the account is body-less.
	View(t, bvnDb, func(batch *database.Batch) {
		stored, err := batch.BPT().Get(record.NewKey("Account", orphan))
		require.NoError(t, err)
		require.Equal(t, plantedFake[:], stored)

		_, mainErr := batch.Account(orphan).Main().Get()
		require.Error(t, mainErr, "kmutt.acme should be body-less")
	})

	activateCyclopsBptRepair(t, sim)

	// After repair, the leaf should equal account.Hash() over the
	// (still body-less) state — i.e., the protocol's empty-state hash.
	stored, computed := readLeafAndHash(t, sim, orphan)
	require.NotEqual(t, plantedFake, stored,
		"orphan leaf should no longer be the planted fake")
	require.Equal(t, computed, stored,
		"orphan stored leaf should equal recomputed empty-state account.Hash()")
}
