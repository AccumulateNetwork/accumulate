// Copyright 2023 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package e2e

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"
	"gitlab.com/accumulatenetwork/accumulate/internal/core"
	"gitlab.com/accumulatenetwork/accumulate/pkg/build"
	"gitlab.com/accumulatenetwork/accumulate/pkg/types/messaging"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
	. "gitlab.com/accumulatenetwork/accumulate/protocol"
	. "gitlab.com/accumulatenetwork/accumulate/test/harness"
	. "gitlab.com/accumulatenetwork/accumulate/test/helpers"
	"gitlab.com/accumulatenetwork/accumulate/test/simulator"
	acctesting "gitlab.com/accumulatenetwork/accumulate/test/testing"
)

func TestMissingSynthTxn(t *testing.T) {
	Run(t, map[string]ExecutorVersion{
		"v1":     ExecutorVersionV1SignatureAnchoring,
		"latest": ExecutorVersionLatest,
	}, func(t *testing.T, version ExecutorVersion) {
		var timestamp uint64

		// Initialize
		globals := new(core.GlobalValues)
		globals.ExecutorVersion = version
		sim := NewSim(t,
			simulator.MemoryDatabase,
			simulator.SimpleNetwork(t.Name(), 3, 3),
			simulator.GenesisWith(GenesisTime, globals),
		)

		alice := acctesting.GenerateKey("Alice")
		aliceUrl := acctesting.AcmeLiteAddressStdPriv(alice)
		bob := acctesting.GenerateKey("Bob")
		bobUrl := acctesting.AcmeLiteAddressStdPriv(bob)
		MakeLiteTokenAccount(t, sim.DatabaseFor(aliceUrl), alice[32:], AcmeUrl())

		// The first time an envelope contains a deposit, drop the first deposit
		var didDrop bool
		sim.SetSubmitHookFor(bobUrl, func(messages []messaging.Message) (drop bool, keepHook bool) {
			for _, msg := range messages {
			again:
				switch m := msg.(type) {
				case *messaging.SyntheticMessage:
					msg = m.Message
					goto again
				case messaging.MessageWithTransaction:
					if m.GetTransaction().Body.Type() == TransactionTypeSyntheticDepositTokens {
						fmt.Printf("Dropping %X\n", m.GetTransaction().GetHash()[:4])
						didDrop = true
						return true, false
					}
				}
			}
			return false, true
		})

		// Execute
		st := make([]*protocol.TransactionStatus, 5)
		for i := range st {
			st[i] = sim.SubmitSuccessfully(MustBuild(t,
				build.Transaction().For(aliceUrl).
					SendTokens(1, protocol.AcmePrecisionPower).To(bobUrl).
					SignWith(aliceUrl).Version(1).Timestamp(&timestamp).PrivateKey(alice)))
		}
		sim.StepUntil(func(*Harness) bool { return didDrop })

		for _, st := range st {
			sim.StepUntil(
				Txn(st.TxID).Succeeds(),
				Txn(st.TxID).Produced().Succeeds())
		}

		// Verify
		lta := GetAccount[*LiteTokenAccount](t, sim.DatabaseFor(bobUrl), bobUrl)
		require.Equal(t, len(st)*protocol.AcmePrecision, int(lta.Balance.Uint64()))
	})
}
