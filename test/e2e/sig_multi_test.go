// Copyright 2023 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package e2e

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
	"gitlab.com/accumulatenetwork/accumulate/pkg/build"
	"gitlab.com/accumulatenetwork/accumulate/pkg/errors"
	. "gitlab.com/accumulatenetwork/accumulate/protocol"
	. "gitlab.com/accumulatenetwork/accumulate/test/harness"
	. "gitlab.com/accumulatenetwork/accumulate/test/helpers"
	"gitlab.com/accumulatenetwork/accumulate/test/simulator"
	acctesting "gitlab.com/accumulatenetwork/accumulate/test/testing"
)

func TestSimpleMultisig(t *testing.T) {
	// Tests AC-3069
	alice := AccountUrl("alice")
	aliceKey := acctesting.GenerateKey(alice, 1)
	bobKey := acctesting.GenerateKey(alice, 2)
	charlieKey := acctesting.GenerateKey(alice, 3)

	// Initialize
	sim := NewSim(t,
		simulator.MemoryDatabase,
		simulator.SimpleNetwork(t.Name(), 3, 1),
		simulator.Genesis(GenesisTime),
	)

	MakeIdentity(t, sim.DatabaseFor(alice), alice, aliceKey[32:], bobKey[32:], charlieKey[32:])
	CreditCredits(t, sim.DatabaseFor(alice), alice.JoinPath("book", "1"), 1e12)
	UpdateAccount(t, sim.DatabaseFor(alice), alice.JoinPath("book", "1"), func(p *KeyPage) {
		p.AcceptThreshold = 2
	})

	// Initiate
	st := sim.BuildAndSubmitSuccessfully(
		build.Transaction().For(alice).
			CreateTokenAccount(alice, "tokens").ForToken(ACME).
			SignWith(alice, "book", "1").Version(1).Timestamp(1).PrivateKey(aliceKey))

	// Wait until the principal lists the transaction as pending
	sim.StepUntil(True(func(h *Harness) bool {
		r, err := h.Query().QueryPendingIds(context.Background(), st[0].TxID.Account(), nil)
		switch {
		case err == nil:
			for _, r := range r.Records {
				if r.Value.Hash() == st[0].TxID.Hash() {
					return true
				}
			}
		case !errors.Is(err, errors.NotFound):
			require.NoError(h.TB, err)
		}
		return false
	}))

	// Sign again
	st = sim.BuildAndSubmitSuccessfully(
		build.SignatureForTxID(st[0].TxID).Load(sim.Query()).
			Url(alice, "book", "1").Version(1).Timestamp(1).PrivateKey(bobKey))

	sim.StepUntil(
		Txn(st[0].TxID).Succeeds())
}
