package e2e

import (
	"math/big"
	"testing"

	"github.com/stretchr/testify/require"
	"gitlab.com/accumulatenetwork/accumulate/internal/database"
	"gitlab.com/accumulatenetwork/accumulate/pkg/build"
	"gitlab.com/accumulatenetwork/accumulate/pkg/types/network"
	. "gitlab.com/accumulatenetwork/accumulate/protocol"
	. "gitlab.com/accumulatenetwork/accumulate/test/harness"
	. "gitlab.com/accumulatenetwork/accumulate/test/helpers"
	"gitlab.com/accumulatenetwork/accumulate/test/simulator"
	acctesting "gitlab.com/accumulatenetwork/accumulate/test/testing"
)

// Does AnchorEmptyBlocks keep a network ticking forever, or does the cascade
// die out too? The answer decides whether it is a heartbeat or just a longer
// tail.
func perpetual(t *testing.T, anchorEmpty bool) {
	liteKey := acctesting.GenerateKey(t.Name())
	lite := acctesting.AcmeLiteAddressStdPriv(liteKey).RootIdentity().JoinPath(ACME)
	g := new(network.GlobalValues)
	g.Globals = new(NetworkGlobals)
	g.Globals.AnchorEmptyBlocks = anchorEmpty
	g.ExecutorVersion = ExecutorVersionV2Kourou
	sim := NewSim(t, simulator.SimpleNetwork(t.Name(), 3, 1), simulator.GenesisWith(GenesisTime, g))
	MakeLiteTokenAccount(t, sim.DatabaseFor(lite), liteKey[32:], AcmeUrl())
	CreditCredits(t, sim.DatabaseFor(lite), lite.RootIdentity(), 1e9)
	CreditTokens(t, sim.DatabaseFor(lite), lite, big.NewInt(1e12))
	sim.StepN(10)

	other := acctesting.AcmeLiteAddressStdPriv(acctesting.GenerateKey("perp-other")).RootIdentity().JoinPath(ACME)
	st := sim.BuildAndSubmitTxnSuccessfully(build.Transaction().For(lite).
		SendTokens(1, 0).To(other).SignWith(lite.RootIdentity()).
		Version(1).Timestamp(1).PrivateKey(liteKey))
	sim.StepUntil(Txn(st.TxID).Succeeds())

	// Blocks and anchors are different costs. The heartbeat's rate limit caps
	// anchors; blocks are driven by the cascade, so measure both.
	stat := func() (blocks, anchors uint64) {
		View(t, sim.Database("BVN0"), func(b *database.Batch) {
			var l *SystemLedger
			require.NoError(t, b.Account(PartitionUrl("BVN0").JoinPath(Ledger)).Main().GetAs(&l))
			blocks = l.Index
			var a *AnchorLedger
			require.NoError(t, b.Account(PartitionUrl("BVN0").JoinPath(AnchorPool)).Main().GetAs(&a))
			anchors = a.MinorBlockSequenceNumber
		})
		return
	}

	sim.StepN(100)
	b0, a0 := stat()
	sim.StepN(200)
	b1, a1 := stat()
	t.Logf("AnchorEmptyBlocks=%v: over 200 idle steps BVN0 produced %d blocks and sent %d anchors (%.2f anchors/block)",
		anchorEmpty, b1-b0, a1-a0, float64(a1-a0)/float64(b1-b0))
}

func TestPerpetual_Default(t *testing.T)           { perpetual(t, false) }
func TestPerpetual_AnchorEmptyBlocks(t *testing.T) { perpetual(t, true) }
