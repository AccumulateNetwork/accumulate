// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package e2e

import (
	"testing"

	"github.com/stretchr/testify/require"
	"gitlab.com/accumulatenetwork/accumulate/pkg/build"
	"gitlab.com/accumulatenetwork/accumulate/pkg/url"
	. "gitlab.com/accumulatenetwork/accumulate/protocol"
	. "gitlab.com/accumulatenetwork/accumulate/test/harness"
	. "gitlab.com/accumulatenetwork/accumulate/test/helpers"
	"gitlab.com/accumulatenetwork/accumulate/test/simulator"
	acctesting "gitlab.com/accumulatenetwork/accumulate/test/testing"
)

// #4148, through the whole pipeline: below Tanegashima the network refuses
// SetLiteAccountDelegate; at Tanegashima it executes and the delegate lands.
// The success case doubles as the registration check — a type that is gated
// but unreachable, or reachable but ungated, fails one of the two halves.
func TestSetLiteAccountDelegate_VersionGate(t *testing.T) {
	setup := func(version ExecutorVersion) (*Sim, *url.URL, []byte, *url.URL) {
		sim := NewSim(t,
			simulator.SimpleNetwork(t.Name()+"-"+version.String(), 1, 1),
			simulator.GenesisWithVersion(GenesisTime, version),
		)
		liteKey := acctesting.GenerateKey("lite", version.String())
		lite := acctesting.AcmeLiteAddressStdPriv(liteKey)
		MakeLiteTokenAccount(t, sim.DatabaseFor(lite), liteKey[32:], AcmeUrl())

		alice := AccountUrl("alice")
		aliceKey := acctesting.GenerateKey(alice)
		MakeIdentity(t, sim.DatabaseFor(alice), alice, aliceKey[32:])

		return sim, lite, liteKey, alice
	}

	t.Run("RejectedBelowActivation", func(t *testing.T) {
		sim, lite, liteKey, alice := setup(ExecutorVersionV2Jiuquan)

		st := sim.SubmitTxn(MustBuild(t,
			build.Transaction().For(lite).
				Body(&SetLiteAccountDelegate{Delegate: alice.JoinPath("book")}).
				SignWith(lite).Version(1).Timestamp(1).PrivateKey(liteKey)))
		if st.Error == nil {
			sim.StepUntil(Txn(st.TxID).Capture(&st).Fails())
		}
		require.Error(t, st.AsError())
		require.ErrorContains(t, st.AsError(), "requires protocol version v2-tanegashima")

		require.Nil(t, GetAccount[*LiteTokenAccount](t, sim.DatabaseFor(lite), lite).Delegate,
			"the refused transaction must not have touched the account")
	})

	t.Run("AllowedAtActivationVersion", func(t *testing.T) {
		sim, lite, liteKey, alice := setup(ExecutorVersionV2Tanegashima)

		st := sim.BuildAndSubmitTxnSuccessfully(
			build.Transaction().For(lite).
				Body(&SetLiteAccountDelegate{Delegate: alice.JoinPath("book")}).
				SignWith(lite).Version(1).Timestamp(1).PrivateKey(liteKey))
		sim.StepUntil(Txn(st.TxID).Completes())

		delegate := GetAccount[*LiteTokenAccount](t, sim.DatabaseFor(lite), lite).Delegate
		require.NotNil(t, delegate)
		require.True(t, delegate.Equal(alice.JoinPath("book")),
			"at Tanegashima the type is registered, reachable, and effective")
	})
}
