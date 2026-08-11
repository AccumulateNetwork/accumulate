// Copyright 2025 The Accumulate Authors
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
	"gitlab.com/accumulatenetwork/accumulate/internal/api/private"
	dut "gitlab.com/accumulatenetwork/accumulate/internal/api/v3"
	"gitlab.com/accumulatenetwork/accumulate/internal/core"
	"gitlab.com/accumulatenetwork/accumulate/internal/core/events"
	"gitlab.com/accumulatenetwork/accumulate/internal/database"
	"gitlab.com/accumulatenetwork/accumulate/pkg/build"
	"gitlab.com/accumulatenetwork/accumulate/pkg/errors"
	"gitlab.com/accumulatenetwork/accumulate/pkg/types/messaging"
	"gitlab.com/accumulatenetwork/accumulate/pkg/url"
	. "gitlab.com/accumulatenetwork/accumulate/protocol"
	. "gitlab.com/accumulatenetwork/accumulate/test/harness"
	. "gitlab.com/accumulatenetwork/accumulate/test/helpers"
	"gitlab.com/accumulatenetwork/accumulate/test/simulator"
	acctesting "gitlab.com/accumulatenetwork/accumulate/test/testing"
)

func init() {
	acctesting.EnableDebugFeatures()
	acctesting.ConfigureSlog(acctesting.DefaultSlogConfig())
}

func TestSequencer(t *testing.T) {
	logger := acctesting.NewTestLogger(t)
	net := simulator.NewSimpleNetwork(t.Name(), 2, 1)
	sim := NewSim(t,
		simulator.WithNetwork(net),
		simulator.GenesisWith(GenesisTime, new(core.GlobalValues)), // Use v1
	)

	aliceKey := acctesting.GenerateKey("alice")
	bobKey := acctesting.GenerateKey("bob")
	alice := acctesting.AcmeLiteAddressStdPriv(aliceKey)
	bob := acctesting.AcmeLiteAddressStdPriv(bobKey)
	sim.SetRoute(alice, "BVN0")
	sim.SetRoute(bob, "BVN1")

	g := new(core.GlobalValues)
	require.NoError(t, g.Load(PartitionUrl("BVN0"), func(account *url.URL, target interface{}) error {
		return sim.DatabaseFor(alice).View(func(batch *database.Batch) error {
			return batch.Account(account).Main().GetAs(target)
		})
	}))

	MakeLiteTokenAccount(t, sim.DatabaseFor(alice), aliceKey[32:], AcmeUrl())
	CreditCredits(t, sim.DatabaseFor(alice), alice.RootIdentity(), 1e9)
	CreditTokens(t, sim.DatabaseFor(alice), alice, big.NewInt(1e12))

	st := sim.SubmitTxnSuccessfully(MustBuild(t,
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

	anchor, err := svc.Sequence(context.Background(), PartitionUrl("BVN0").JoinPath(AnchorPool), DnUrl().JoinPath(AnchorPool), 1, private.SequenceOptions{})
	require.NoError(t, err)
	require.IsType(t, (*messaging.TransactionMessage)(nil), anchor.Message)
	require.IsType(t, (*BlockValidatorAnchor)(nil), anchor.Message.(*messaging.TransactionMessage).Transaction.Body)
	require.Len(t, anchor.Signatures.Records, 1)
	sigs := anchor.Signatures.Records[0].Signatures.Records
	require.Len(t, sigs, 2)
	require.IsType(t, (*PartitionSignature)(nil), sigs[0].Message.(*messaging.SignatureMessage).Signature)
	require.IsType(t, (*ED25519Signature)(nil), sigs[1].Message.(*messaging.SignatureMessage).Signature)

	synth, err := svc.Sequence(context.Background(), PartitionUrl("BVN0").JoinPath(Synthetic), PartitionUrl("BVN1").JoinPath(Synthetic), 1, private.SequenceOptions{})
	require.NoError(t, err)
	require.IsType(t, (*messaging.TransactionMessage)(nil), anchor.Message)
	require.IsType(t, (*SyntheticDepositTokens)(nil), synth.Message.(*messaging.TransactionMessage).Transaction.Body)
	require.Len(t, synth.Signatures.Records, 1)
	sigs = synth.Signatures.Records[0].Signatures.Records
	require.Len(t, sigs, 3)
	require.IsType(t, (*PartitionSignature)(nil), sigs[0].Message.(*messaging.SignatureMessage).Signature)
	require.IsType(t, (*ReceiptSignature)(nil), sigs[1].Message.(*messaging.SignatureMessage).Signature)
	require.IsType(t, (*ED25519Signature)(nil), sigs[2].Message.(*messaging.SignatureMessage).Signature)

	// A sequencer whose key is NOT an active validator of the source partition
	// must DECLINE, rather than re-sign the message with a key the destination
	// will reject as "not an active validator" — which silently and permanently
	// wedges the receiver's synthetic stream (#4070). A non-validator node (seed,
	// bootstrap, or follower) running the sequencer service can serve the data
	// but cannot produce a usable signature, so it must return an error, letting
	// the caller route the pull to a peer that IS a validator.
	notAValidator := acctesting.GenerateKey("not-a-validator")
	badSvc := dut.NewSequencer(dut.SequencerParams{
		Logger:       logger,
		Database:     sim.DatabaseFor(alice),
		EventBus:     events.NewBus(logger),
		Globals:      g,
		Partition:    "BVN0",
		ValidatorKey: notAValidator, // 64-byte ed25519 private key; pubkey is not in the validator set
	})

	_, err = badSvc.Sequence(context.Background(), PartitionUrl("BVN0").JoinPath(Synthetic), PartitionUrl("BVN1").JoinPath(Synthetic), 1, private.SequenceOptions{})
	require.Error(t, err)
	require.Contains(t, err.Error(), "not an active validator")

	// Requesting a sequence number the source has NOT produced yet must return a
	// clean NotFound, not a generic load error. The tail-probe healer relies on
	// this to tell "the stream is caught up" from a real failure: a synthetic
	// dropped at the tail leaves no visible gap, so the receiver speculatively
	// asks for Delivered+1, and NotFound is the answer when nothing more exists
	// to send (#4070). Only one synthetic (seq 1) was produced above, so seq 2
	// is beyond the produced head.
	_, err = svc.Sequence(context.Background(), PartitionUrl("BVN0").JoinPath(Synthetic), PartitionUrl("BVN1").JoinPath(Synthetic), 2, private.SequenceOptions{})
	require.Error(t, err)
	require.True(t, errors.Is(err, errors.NotFound), "expected NotFound, got %v", err)
}
