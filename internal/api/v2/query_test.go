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
	"gitlab.com/accumulatenetwork/accumulate/internal/api/v2"
	"gitlab.com/accumulatenetwork/accumulate/pkg/build"
	. "gitlab.com/accumulatenetwork/accumulate/protocol"
	. "gitlab.com/accumulatenetwork/accumulate/test/harness"
	. "gitlab.com/accumulatenetwork/accumulate/test/helpers"
	"gitlab.com/accumulatenetwork/accumulate/test/simulator"
	acctesting "gitlab.com/accumulatenetwork/accumulate/test/testing"
)

func init() {
	acctesting.EnableDebugFeatures()
}

func TestQuerySendTokens(t *testing.T) {
	// Tests https://gitlab.com/accumulatenetwork/accumulate/-/issues/3290

	sim := NewSim(t,
		simulator.SimpleNetwork(t.Name(), 3, 3),
		simulator.Genesis(GenesisTime),
	)

	aliceKey := acctesting.GenerateKey("alice")
	bobKey := acctesting.GenerateKey("bob")
	alice := acctesting.AcmeLiteAddressStdPriv(aliceKey)
	bob := acctesting.AcmeLiteAddressStdPriv(bobKey)

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

	c := sim.S.ClientV2(Directory)
	_, err := c.QueryTx(context.Background(), &api.TxnQuery{TxIdUrl: st.TxID})
	require.NoError(t, err)
}
