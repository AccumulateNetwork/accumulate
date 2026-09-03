// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package e2e

import (
	"strconv"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gitlab.com/accumulatenetwork/accumulate/internal/database"
	"gitlab.com/accumulatenetwork/accumulate/internal/database/indexing"
	"gitlab.com/accumulatenetwork/accumulate/pkg/build"
	"gitlab.com/accumulatenetwork/accumulate/pkg/errors"
	. "gitlab.com/accumulatenetwork/accumulate/protocol"
	. "gitlab.com/accumulatenetwork/accumulate/test/harness"
	. "gitlab.com/accumulatenetwork/accumulate/test/helpers"
	"gitlab.com/accumulatenetwork/accumulate/test/simulator"
	acctesting "gitlab.com/accumulatenetwork/accumulate/test/testing"
)

// Before Jiuquan every block writes a BlockLedger ACCOUNT — one BPT entry per
// block, permanently. From Jiuquan on, a block writes one record keyed by its
// index and puts the record's hash on the ledger's block-ledger chain (executor
// spec, "The block ledger"). Nothing is migrated at activation: old blocks stay
// as accounts and reads fall through to them. These tests run the real executor
// through real blocks and check which form lands in the database.

// blockLedgerForms scans blocks [1, head] on the partition and reports which
// blocks have the account form and which have a block ledger record.
func blockLedgerForms(t *testing.T, db database.Viewer, partition string) (accounts, records []uint64, head uint64) {
	t.Helper()
	View(t, db, func(batch *database.Batch) {
		partUrl := PartitionUrl(partition)
		var system *SystemLedger
		require.NoError(t, batch.Account(partUrl.JoinPath(Ledger)).Main().GetAs(&system))
		head = system.Index

		ledger := batch.Account(partUrl.JoinPath(Ledger))
		for i := uint64(1); i <= head; i++ {
			var bl *BlockLedger
			err := batch.Account(partUrl.JoinPath(Ledger, strconv.FormatUint(i, 10))).Main().GetAs(&bl)
			switch {
			case err == nil:
				accounts = append(accounts, i)
			case errors.Is(err, errors.NotFound):
				// no account form
			default:
				require.NoError(t, err)
			}

			_, err = ledger.BlockLedger(i).Get()
			switch {
			case err == nil:
				records = append(records, i)
			case errors.Is(err, errors.NotFound):
				// no record
			default:
				require.NoError(t, err)
			}
		}
	})
	return accounts, records, head
}

func TestBlockLedger_PreJiuquanWritesABlockLedgerAccount(t *testing.T) {
	sim := NewSim(t,
		simulator.SimpleNetwork(t.Name(), 1, 1),
		simulator.GenesisWithVersion(GenesisTime, ExecutorVersionV2Vandenberg),
	)
	sim.StepN(10)

	accounts, logs, head := blockLedgerForms(t, sim.Database("BVN0"), "BVN0")
	require.NotZero(t, head)
	require.NotEmpty(t, accounts, "pre-Jiuquan, blocks must record a BlockLedger account")
	assert.Empty(t, logs, "pre-Jiuquan, nothing may be written to the indexing log")

	// And the account takes a BPT entry — the cost this issue exists to stop.
	View(t, sim.Database("BVN0"), func(batch *database.Batch) {
		u := PartitionUrl("BVN0").JoinPath(Ledger, strconv.FormatUint(accounts[0], 10))
		_, err := batch.BPT().Get(batch.Account(u).Key())
		assert.NoError(t, err, "the pre-Jiuquan block ledger account occupies the state tree")
	})
}

func TestBlockLedger_JiuquanWritesTheRecordAndChain(t *testing.T) {
	sim := NewSim(t,
		simulator.SimpleNetwork(t.Name(), 1, 1),
		simulator.GenesisWithVersion(GenesisTime, ExecutorVersionV2Jiuquan),
	)
	sim.StepN(10)

	accounts, records, head := blockLedgerForms(t, sim.Database("BVN0"), "BVN0")
	require.NotZero(t, head)
	require.NotEmpty(t, records, "under Jiuquan, blocks must write a block ledger record")

	// And the chain carries one entry per recorded block, so the ledger
	// account's hash commits to what every block changed.
	View(t, sim.Database("BVN0"), func(batch *database.Batch) {
		chain, err := batch.Account(PartitionUrl("BVN0").JoinPath(Ledger)).BlockLedgerChain().Get()
		require.NoError(t, err)
		assert.Equal(t, int64(len(records)), chain.Height(), "one block-ledger chain entry per recorded block")
	})

	// Characterization: the GENESIS block still writes the account form, even
	// when the genesis version is Jiuquan or later — the version is activated
	// during genesis, so block 1's Close sees it as not yet enabled. Every
	// block after genesis uses the log.
	assert.Equal(t, []uint64{1}, accounts,
		"only the genesis block may create a BlockLedger account under Jiuquan")
}

// The step-1 verification from #4147: a chain that has run at or past Jiuquan
// from genesis — the DAG-BFT line runs Kourou — has only ever written the log
// form, with ONE exception this test pins: the genesis block itself writes
// the account form on every partition, because the version activates during
// genesis. So "no BlockLedger accounts exist in a DAG-BFT database" is
// actually "exactly one per partition, at ledger/1". (The corresponding check
// against a live DAG-BFT database is operational, not something a simulator
// test can prove.)
func TestBlockLedger_OnlyGenesisWritesAnAccountOnAChainGenesisedAtKourou(t *testing.T) {
	sim := NewSim(t,
		simulator.SimpleNetwork(t.Name(), 2, 1),
		simulator.GenesisWithVersion(GenesisTime, ExecutorVersionLatest),
	)
	sim.StepN(20)

	for _, part := range []string{Directory, "BVN0", "BVN1"} {
		accounts, _, head := blockLedgerForms(t, sim.Database(part), part)
		require.NotZero(t, head)
		assert.Equal(t, []uint64{1}, accounts,
			"%s: a chain that never ran pre-Jiuquan holds exactly the genesis block's account", part)
	}
}

// The move from account to record changes WHERE block entries are recorded, not
// WHAT: the same transaction produces the same chain-update entries in both
// forms. Runs identical traffic under Vandenberg and under Jiuquan and
// compares what was recorded about it, through the same read path.
func TestBlockLedger_RecordCarriesTheSameChainUpdates(t *testing.T) {
	recordFor := func(version ExecutorVersion) map[string]bool {
		sim := NewSim(t,
			simulator.SimpleNetwork(t.Name()+"-"+version.String(), 1, 1),
			simulator.GenesisWithVersion(GenesisTime, version),
		)

		aliceKey := acctesting.GenerateKey("alice")
		alice := acctesting.AcmeLiteAddressStdPriv(aliceKey)
		MakeLiteTokenAccount(t, sim.DatabaseFor(alice), aliceKey[32:], AcmeUrl())

		st := sim.BuildAndSubmitTxnSuccessfully(
			build.Transaction().For(alice).
				BurnTokens(1, 0).
				SignWith(alice).Version(1).Timestamp(1).PrivateKey(aliceKey))
		sim.StepUntil(Txn(st.TxID).Completes())

		// Collect every (account, chain) recorded about alice, whichever form
		// recorded it.
		recorded := map[string]bool{}
		View(t, sim.Database("BVN0"), func(batch *database.Batch) {
			ledger := batch.Account(PartitionUrl("BVN0").JoinPath(Ledger))
			var system *SystemLedger
			require.NoError(t, batch.Account(PartitionUrl("BVN0").JoinPath(Ledger)).Main().GetAs(&system))
			for i := uint64(1); i <= system.Index; i++ {
				_, entries, err := indexing.LoadBlockLedger(ledger, i)
				if err != nil {
					continue
				}
				for _, e := range entries {
					if e.Account.RootIdentity().Equal(alice.RootIdentity()) {
						recorded[e.Account.ShortString()+";"+e.Chain] = true
					}
				}
			}
		})
		return recorded
	}

	pre := recordFor(ExecutorVersionV2Vandenberg)
	post := recordFor(ExecutorVersionV2Jiuquan)
	require.NotEmpty(t, pre, "the burn must appear in the pre-Jiuquan record")
	assert.Equal(t, pre, post,
		"the record must carry exactly the chain updates the account form recorded — the move changes the container, not the content")
}

// The activation itself, end to end: historical block queries answer the same
// before and after, the old accounts are left exactly where they were — in the
// database AND in the BPT, because nothing is migrated (executor spec, "The
// block ledger", activation and history) — and new blocks write the record.
func TestBlockLedger_HistoricalBlockQueryWorksAcrossActivation(t *testing.T) {
	sim := NewSim(t,
		simulator.SimpleNetwork(t.Name(), 1, 1),
		simulator.GenesisWithVersion(GenesisTime, ExecutorVersionV2Vandenberg),
	)
	sim.StepN(10)

	// Record every pre-activation block's ledger, via the same read path the
	// API's block query uses.
	before := map[uint64][]*BlockEntry{}
	var preHead uint64
	View(t, sim.Database("BVN0"), func(batch *database.Batch) {
		ledger := batch.Account(PartitionUrl("BVN0").JoinPath(Ledger))
		var system *SystemLedger
		require.NoError(t, batch.Account(PartitionUrl("BVN0").JoinPath(Ledger)).Main().GetAs(&system))
		preHead = system.Index
		for i := uint64(1); i <= preHead; i++ {
			_, entries, err := indexing.LoadBlockLedger(ledger, i)
			if err == nil {
				before[i] = entries
			}
		}
	})
	require.NotEmpty(t, before, "precondition: some blocks were recorded before activation")

	// Activate Jiuquan.
	st := sim.SubmitTxnSuccessfully(MustBuild(t,
		build.Transaction().For(DnUrl()).
			ActivateProtocolVersion(ExecutorVersionV2Jiuquan).
			SignWith(DnUrl(), Operators, "1").Version(1).Timestamp(1).Signer(sim.SignWithNode(Directory, 0))))
	sim.StepUntil(Txn(st.TxID).Succeeds())
	sim.StepN(15) // let the anchor propagate and the BVN cross the transition

	// The BVN crosses the transition when the DN's anchor arrives, several
	// blocks after preHead. Every account-form block up to and including the
	// transition block wrote the old form; the LAST account-form block is the
	// transition block itself (its Close still ran with Vandenberg active).
	accounts, records, _ := blockLedgerForms(t, sim.Database("BVN0"), "BVN0")
	require.NotEmpty(t, accounts)
	require.NotEmpty(t, records, "post-activation blocks must write the record")
	transition := accounts[len(accounts)-1]
	require.Greater(t, transition, preHead, "the transition lands after the pre-activation snapshot")

	View(t, sim.Database("BVN0"), func(batch *database.Batch) {
		ledger := batch.Account(PartitionUrl("BVN0").JoinPath(Ledger))

		// Confirm the BVN actually crossed the transition.
		var system *SystemLedger
		require.NoError(t, batch.Account(PartitionUrl("BVN0").JoinPath(Ledger)).Main().GetAs(&system))
		require.Equal(t, ExecutorVersionV2Jiuquan, system.ExecutorVersion,
			"precondition: the BVN must have activated Jiuquan")

		for i, want := range before {
			// Same answer as before activation.
			_, got, err := indexing.LoadBlockLedger(ledger, i)
			require.NoError(t, err, "block %d must still be readable after activation", i)
			require.Equal(t, len(want), len(got), "block %d's entries must survive activation", i)

			// Nothing was migrated: the account is still there, and so is
			// its BPT entry. Removing them is a separate reorganization.
			u := PartitionUrl("BVN0").JoinPath(Ledger, strconv.FormatUint(i, 10))
			var bl *BlockLedger
			require.NoError(t, batch.Account(u).Main().GetAs(&bl),
				"activation does not touch the pre-activation accounts")
			_, err = batch.BPT().Get(batch.Account(u).Key())
			assert.NoError(t, err, "block %d's BPT entry is left in place — there is no migration", i)
		}

		// Blocks after the transition never create accounts.
		for i := transition + 1; i <= system.Index; i++ {
			u := PartitionUrl("BVN0").JoinPath(Ledger, strconv.FormatUint(i, 10))
			var bl *BlockLedger
			err := batch.Account(u).Main().GetAs(&bl)
			assert.True(t, errors.Is(err, errors.NotFound),
				"post-transition block %d must not create a BlockLedger account", i)
		}
	})
}
