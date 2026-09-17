package e2e

import (
	"context"
	"math/big"
	"testing"

	"github.com/stretchr/testify/require"
	apiv3 "gitlab.com/accumulatenetwork/accumulate/pkg/api/v3"
	"gitlab.com/accumulatenetwork/accumulate/pkg/build"
	"gitlab.com/accumulatenetwork/accumulate/pkg/types/network"
	. "gitlab.com/accumulatenetwork/accumulate/protocol"
	. "gitlab.com/accumulatenetwork/accumulate/test/harness"
	. "gitlab.com/accumulatenetwork/accumulate/test/helpers"
	"gitlab.com/accumulatenetwork/accumulate/test/simulator"
	acctesting "gitlab.com/accumulatenetwork/accumulate/test/testing"
)

// Does AnchorEmptyBlocks -- the heartbeat the protocol already has -- make a
// quiet network's roots bindable, now that call 2 extends through the bpt chain?
func quietLiveness(t *testing.T, anchorEmpty bool) {
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

	// Some activity, then leave it alone
	other := acctesting.AcmeLiteAddressStdPriv(acctesting.GenerateKey("ql-other")).RootIdentity().JoinPath(ACME)
	st := sim.BuildAndSubmitTxnSuccessfully(
		build.Transaction().For(lite).SendTokens(1, 0).To(other).
			SignWith(lite.RootIdentity()).Version(1).Timestamp(1).PrivateKey(liteKey))
	sim.StepUntil(Txn(st.TxID).Succeeds())
	sim.StepN(30)

	// A reader shows up during the quiet period
	first := sim.QueryAccount(lite, &apiv3.DefaultQuery{IncludeReceipt: &apiv3.ReceiptOptions{ForAny: true}})
	var root [32]byte
	copy(root[:], first.Receipt.Receipt.Anchor)
	svc := proofServiceFor(sim, first.Receipt.Partition)

	var rec *apiv3.AnchorReceiptRecord
	var err error
	for i := 0; i < 200 && (rec == nil || !rec.Anchored); i++ {
		sim.Step()
		rec, err = svc.AnchorReceipt(context.Background(), apiv3.AnchorReceiptOptions{
			Partition: first.Receipt.Partition, BptRoot: root})
		require.NoError(t, err)
	}
	if rec.Anchored {
		joined, err := first.Receipt.Receipt.Combine(rec.Receipt)
		require.NoError(t, err)
		require.True(t, joined.Validate(nil))
		t.Logf("AnchorEmptyBlocks=%v: the reader's root BOUND at DN block %d, no transaction needed",
			anchorEmpty, rec.DirectoryBlock)
	} else {
		t.Logf("AnchorEmptyBlocks=%v: the reader's root never bound in 200 steps -- stuck until someone transacts",
			anchorEmpty)
	}
}

func TestQuietLiveness_Default(t *testing.T)           { quietLiveness(t, false) }
func TestQuietLiveness_AnchorEmptyBlocks(t *testing.T) { quietLiveness(t, true) }
