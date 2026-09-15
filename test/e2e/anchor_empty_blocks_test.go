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

	h := func() uint64 {
		var n uint64
		View(t, sim.Database(Directory), func(b *database.Batch) {
			var l *SystemLedger
			require.NoError(t, b.Account(DnUrl().JoinPath(Ledger)).Main().GetAs(&l))
			n = l.Index
		})
		return n
	}

	sim.StepN(100)
	a := h()
	sim.StepN(100)
	b := h()
	sim.StepN(200)
	c := h()
	t.Logf("AnchorEmptyBlocks=%v: DN height after settling %d -> %d -> %d (+%d then +%d)",
		anchorEmpty, a, b, c, b-a, c-b)
}

func TestPerpetual_Default(t *testing.T)           { perpetual(t, false) }
func TestPerpetual_AnchorEmptyBlocks(t *testing.T) { perpetual(t, true) }
