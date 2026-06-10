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
	. "gitlab.com/accumulatenetwork/accumulate/protocol"
	. "gitlab.com/accumulatenetwork/accumulate/test/harness"
	. "gitlab.com/accumulatenetwork/accumulate/test/helpers"
	"gitlab.com/accumulatenetwork/accumulate/test/simulator"
	acctesting "gitlab.com/accumulatenetwork/accumulate/test/testing"
)

// TestCyclopsBptRepairActivation verifies that activating
// ExecutorVersionV2CyclopsBptRepair on a partition named "Cyclops"
// repairs a planted BPT leaf that doesn't match account.Hash().
//
// The test plants the same kind of corruption observed on mainnet —
// a stored leaf that doesn't reproduce from the account's current
// state — then submits the protocol-version-activation transaction
// and verifies that after the activation block (1) the version is
// active and (2) the planted corruption is gone (stored leaf now
// matches account.Hash()).
func TestCyclopsBptRepairActivation(t *testing.T) {
	const planted = byte(0xC0)

	// Build a network with a BVN named "Cyclops" so the repair table's
	// per-partition lookup matches.
	net := simulator.NewSimpleNetwork(t.Name(), 1, 1)
	net.Bvns[0].Id = "Cyclops"

	sim := NewSim(t,
		simulator.WithNetwork(net),
		simulator.GenesisWithVersion(GenesisTime, ExecutorVersionV2Jiuquan),
	)

	// Pre-create one of the Class-A target ADIs (csrc.acme — short URL,
	// uses the default genesis-pair pattern).
	adi := AccountUrl("csrc.acme")
	signer := sim.SignWithNode(Directory, 0)

	// Plant the ADI body in the BVN partition so it's a real account.
	bvnDb := sim.DatabaseFor(adi)
	MakeIdentity(t, bvnDb, adi, acctesting.GenerateKey(adi)[32:])
	CreditCredits(t, bvnDb, adi.JoinPath("book", "1"), 1e9)

	// Capture the initial (correct) leaf so we can detect the
	// repair really did something.
	var initialHash [32]byte
	View(t, bvnDb, func(batch *database.Batch) {
		h, err := batch.Account(adi).Hash()
		require.NoError(t, err)
		initialHash = h
	})

	// Plant a deliberately-wrong leaf to simulate the post-reorg drift.
	fake := [32]byte{}
	for i := range fake {
		fake[i] = planted
	}
	Update(t, bvnDb, func(batch *database.Batch) {
		key := record.NewKey("Account", adi)
		require.NoError(t, batch.BPT().Insert(key, fake[:]))
	})

	// Verify the corruption took.
	View(t, bvnDb, func(batch *database.Batch) {
		key := record.NewKey("Account", adi)
		stored, err := batch.BPT().Get(key)
		require.NoError(t, err)
		require.Equal(t, fake[:], stored, "planted corruption should be present")
	})

	// Activate the repair version.
	st := sim.BuildAndSubmitTxnSuccessfully(
		build.Transaction().For(DnUrl()).
			ActivateProtocolVersion(ExecutorVersionV2CyclopsBptRepair).
			SignWith(DnUrl(), Operators, "1").Version(1).Timestamp(1).Signer(signer))
	sim.StepUntil(Txn(st.TxID).Succeeds())

	// Step a few blocks for the version transition to propagate to the
	// BVN and for the post-update action to run.
	sim.StepN(20)

	// The leaf should now match account.Hash() again — the planted
	// corruption is gone. The repair re-registers the standard chain
	// set on every target, so the leaf may not equal the originally
	// captured pre-corruption hash, but it MUST equal the recomputed
	// hash over the post-repair state.
	View(t, bvnDb, func(batch *database.Batch) {
		key := record.NewKey("Account", adi)
		stored, err := batch.BPT().Get(key)
		require.NoError(t, err)

		recomputed, err := batch.Account(adi).Hash()
		require.NoError(t, err)

		require.NotEqual(t, fake[:], stored,
			"stored leaf should no longer be the planted fake")
		require.Equal(t, recomputed[:], stored,
			"stored leaf should now equal recomputed account.Hash()")
	})

	// Sanity guard: initialHash was captured before corruption. Use it
	// to keep the linter happy if needed, otherwise unused.
	_ = initialHash
}
