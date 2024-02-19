// Copyright 2024 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package e2e

import (
	"math/big"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gitlab.com/accumulatenetwork/accumulate/internal/core/execute"
	"gitlab.com/accumulatenetwork/accumulate/pkg/build"
	"gitlab.com/accumulatenetwork/accumulate/pkg/types/messaging"
	"gitlab.com/accumulatenetwork/accumulate/pkg/url"
	. "gitlab.com/accumulatenetwork/accumulate/protocol"
	. "gitlab.com/accumulatenetwork/accumulate/test/harness"
	. "gitlab.com/accumulatenetwork/accumulate/test/helpers"
	"gitlab.com/accumulatenetwork/accumulate/test/simulator"
	acctesting "gitlab.com/accumulatenetwork/accumulate/test/testing"
)

func TestCreditAccounting(t *testing.T) {
	cases := []struct {
		Name    string
		Version ExecutorVersion
	}{
		{"V2", ExecutorVersionV2Baikonur},
		{"Latest", ExecutorVersionV2Vandenberg},
	}

	for _, c := range cases {
		t.Run(c.Name, func(t *testing.T) {
			liteKey := acctesting.GenerateKey(t.Name(), "Lite")
			lite := acctesting.AcmeLiteAddress(liteKey[32:])

			// Initialize
			sim := NewSim(t,
				simulator.SimpleNetwork(t.Name(), 1, 1),
				simulator.GenesisWithVersion(GenesisTime, c.Version),
			)

			// Verify that AcmeBurnt is never set
			if c.Version.V2VandenbergEnabled() {
				sim.SetBlockHookFor(lite, func(_ execute.BlockParams, env []*messaging.Envelope) (_ []*messaging.Envelope, keepHook bool) {
					ledger := GetAccount[*SystemLedger](t, sim.DatabaseFor(lite), url.MustParse("bvn-BVN0.acme").JoinPath(Ledger))
					assert.Zero(t, ledger.AcmeBurnt)
					return env, true
				})
			}

			MakeLiteTokenAccount(t, sim.DatabaseFor(lite), liteKey[32:], AcmeUrl())
			CreditCredits(t, sim.DatabaseFor(lite), lite.RootIdentity(), 1e9)
			CreditTokens(t, sim.DatabaseFor(lite), lite, big.NewInt(1e12))

			// Check the balance before
			before := GetAccount[*TokenIssuer](t, sim.Database(Directory), AcmeUrl()).Issued

			// Burn ACME, buy credits
			st := sim.BuildAndSubmitTxnSuccessfully(
				build.Transaction().For(lite).
					AddCredits().Spend(10).To(lite).WithOracle(InitialAcmeOracle).
					SignWith(lite).Version(1).Timestamp(1).PrivateKey(liteKey))

			sim.StepUntil(
				Txn(st.TxID).Succeeds(),
				Txn(st.TxID).Produced().Succeeds())

			// Wait for anchoring to happen
			var after big.Int
			sim.StepUntil(True(func(h *Harness) bool {
				after = GetAccount[*TokenIssuer](t, sim.Database(Directory), AcmeUrl()).Issued
				return after.Cmp(&before) != 0
			}))

			// Verify the burn reached acc://ACME
			require.Equal(t, "10.00000000", FormatBigAmount(before.Sub(&before, &after), AcmePrecisionPower))
		})
	}
}
