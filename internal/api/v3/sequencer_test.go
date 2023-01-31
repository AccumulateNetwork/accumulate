// Copyright 2023 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package api_test

import (
	"context"
	"math/big"
	"testing"

	"github.com/stretchr/testify/require"
	dut "gitlab.com/accumulatenetwork/accumulate/internal/api/v3"
	"gitlab.com/accumulatenetwork/accumulate/internal/core"
	"gitlab.com/accumulatenetwork/accumulate/internal/core/events"
	"gitlab.com/accumulatenetwork/accumulate/internal/database"
	"gitlab.com/accumulatenetwork/accumulate/internal/node/config"
	"gitlab.com/accumulatenetwork/accumulate/pkg/build"
	"gitlab.com/accumulatenetwork/accumulate/pkg/url"
	. "gitlab.com/accumulatenetwork/accumulate/protocol"
	. "gitlab.com/accumulatenetwork/accumulate/test/harness"
	. "gitlab.com/accumulatenetwork/accumulate/test/helpers"
	"gitlab.com/accumulatenetwork/accumulate/test/simulator"
	acctesting "gitlab.com/accumulatenetwork/accumulate/test/testing"
)

func init() {
	acctesting.EnableDebugFeatures()
}

func TestSequencer(t *testing.T) {
	logger := acctesting.NewTestLogger(t)
	net := simulator.SimpleNetwork(t.Name(), 2, 1)
	sim := NewSim(t,
		simulator.MemoryDatabase,
		net,
		simulator.GenesisWith(GenesisTime, new(core.GlobalValues)), // Use v1
	)

	aliceKey := acctesting.GenerateKey("alice")
	bobKey := acctesting.GenerateKey("bob")
	alice := acctesting.AcmeLiteAddressStdPriv(aliceKey)
	bob := acctesting.AcmeLiteAddressStdPriv(bobKey)
	sim.SetRoute(alice, "BVN0")
	sim.SetRoute(bob, "BVN1")

	g := new(core.GlobalValues)
	require.NoError(t, g.Load(config.NetworkUrl{URL: PartitionUrl("BVN0")}, func(account *url.URL, target interface{}) error {
		return sim.DatabaseFor(alice).View(func(batch *database.Batch) error {
			return batch.Account(account).GetStateAs(target)
		})
	}))

	MakeLiteTokenAccount(t, sim.DatabaseFor(alice), aliceKey[32:], AcmeUrl())
	CreditCredits(t, sim.DatabaseFor(alice), alice.RootIdentity(), 1e9)
	CreditTokens(t, sim.DatabaseFor(alice), alice, big.NewInt(1e12))

	st := sim.SubmitSuccessfully(MustBuild(t,
		build.Transaction().For(alice).
			SendTokens(123, 0).To(bob).
			SignWith(alice).Version(1).Timestamp(1).PrivateKey(aliceKey)))

	sim.StepUntil(
		Txn(st.TxID).Succeeds(),
		Txn(st.TxID).Produced().Succeeds())

	svc := dut.NewSequencer(dut.SequencerParams{
		Logger:       logger,
		Database:     sim.DatabaseFor(alice),
		EventBus:     events.NewBus(logger),
		Globals:      g,
		Partition:    "BVN0",
		ValidatorKey: net.Bvns[0].Nodes[0].PrivValKey,
	})

	anchor, err := svc.Sequence(context.Background(), PartitionUrl("BVN0").JoinPath(AnchorPool), DnUrl().JoinPath(AnchorPool), 1)
	require.NoError(t, err)
	require.IsType(t, (*BlockValidatorAnchor)(nil), anchor.Transaction.Body)
	require.Len(t, anchor.Signatures.Records, 1)
	require.IsType(t, (*SignatureSet)(nil), anchor.Signatures.Records[0].Signature)
	sigs := anchor.Signatures.Records[0].Signature.(*SignatureSet).Signatures
	require.Len(t, sigs, 2)
	require.IsType(t, (*PartitionSignature)(nil), sigs[0])
	require.IsType(t, (*ED25519Signature)(nil), sigs[1])

	synth, err := svc.Sequence(context.Background(), PartitionUrl("BVN0").JoinPath(Synthetic), PartitionUrl("BVN1").JoinPath(Synthetic), 1)
	require.NoError(t, err)
	require.IsType(t, (*SyntheticDepositTokens)(nil), synth.Transaction.Body)
	require.Len(t, synth.Signatures.Records, 1)
	require.IsType(t, (*SignatureSet)(nil), synth.Signatures.Records[0].Signature)
	sigs = synth.Signatures.Records[0].Signature.(*SignatureSet).Signatures
	require.Len(t, sigs, 3)
	require.IsType(t, (*PartitionSignature)(nil), sigs[0])
	require.IsType(t, (*ReceiptSignature)(nil), sigs[1])
	require.IsType(t, (*ED25519Signature)(nil), sigs[2])
}
