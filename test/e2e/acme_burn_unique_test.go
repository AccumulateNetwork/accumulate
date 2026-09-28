// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package e2e

import (
	"math/big"
	"testing"

	"github.com/stretchr/testify/require"
	"gitlab.com/accumulatenetwork/accumulate/internal/database"
	"gitlab.com/accumulatenetwork/accumulate/pkg/build"
	"gitlab.com/accumulatenetwork/accumulate/pkg/types/messaging"
	"gitlab.com/accumulatenetwork/accumulate/pkg/url"
	. "gitlab.com/accumulatenetwork/accumulate/protocol"
	. "gitlab.com/accumulatenetwork/accumulate/test/harness"
	. "gitlab.com/accumulatenetwork/accumulate/test/helpers"
	"gitlab.com/accumulatenetwork/accumulate/test/simulator"
	acctesting "gitlab.com/accumulatenetwork/accumulate/test/testing"
)

// TestRepeatedBurnsAllReduceIssued is #4458: a partition's block-end ACME burn
// carried only its amount, so two burns of one amount were one transaction by
// hash, and the second was skipped as already delivered. ACME's issued stayed
// too high by every repeat. Each burn now names the partition and block that
// produced it, so every one executes and lands on acc://ACME's main chain.
func TestRepeatedBurnsAllReduceIssued(t *testing.T) {
	t.Run("on the Directory", func(t *testing.T) { repeatedBurns(t, Directory) })
	t.Run("from a BVN", func(t *testing.T) { repeatedBurns(t, "BVN0") })
}

func repeatedBurns(t *testing.T, buyer string) {
	const purchases = 3
	dave := url.MustParse("dave")
	daveKey := acctesting.GenerateKey(dave)
	sim := NewSim(t,
		simulator.SimpleNetwork(t.Name(), 1, 1),
		simulator.Genesis(GenesisTime),
	)
	sim.SetRoute(dave, buyer)
	MakeIdentity(t, sim.DatabaseFor(dave), dave, daveKey[32:])
	CreditCredits(t, sim.DatabaseFor(dave), dave.JoinPath("book", "1"), 1e12)
	MakeAccount(t, sim.DatabaseFor(dave), &TokenAccount{Url: dave.JoinPath("tokens"), TokenUrl: AcmeUrl()})
	CreditTokens(t, sim.DatabaseFor(dave), dave.JoinPath("tokens"), big.NewInt(1e12))

	issued := func() big.Int {
		return GetAccount[*TokenIssuer](t, sim.Database(Directory), AcmeUrl()).Issued
	}
	start := issued()
	before := start
	for i := 0; i < purchases; i++ {
		st := sim.BuildAndSubmitTxnSuccessfully(
			build.Transaction().For(dave, "tokens").
				AddCredits().Spend(10).To(dave.JoinPath("book", "1")).WithOracle(InitialAcmeOracle).
				SignWith(dave, "book", "1").Version(1).Timestamp(uint64(i + 1)).PrivateKey(daveKey))
		sim.StepUntil(Txn(st.TxID).Succeeds())

		// Wait for the burn to reach acc://ACME; a skipped burn never does
		sim.StepUntilN(50, True(func(h *Harness) bool {
			now := issued()
			return now.Cmp(&before) != 0
		}))
		before = issued()
	}

	// Every purchase is burnt
	drop := new(big.Int).Sub(&start, &before)
	require.Equal(t, "30.00000000", FormatBigAmount(drop, AcmePrecisionPower))

	// Every burn is on acc://ACME's main chain, each a different transaction
	View(t, sim.Database(Directory), func(batch *database.Batch) {
		chain, err := batch.Account(AcmeUrl()).MainChain().Get()
		require.NoError(t, err)
		seen := map[[32]byte]bool{}
		for i := int64(0); i < chain.Height(); i++ {
			h, err := chain.Entry(i)
			require.NoError(t, err)
			msg, err := batch.Message([32]byte(h)).Main().Get()
			require.NoError(t, err)
			txn, ok := msg.(*messaging.TransactionMessage)
			if !ok {
				continue
			}
			if body, ok := txn.Transaction.Body.(*SyntheticBurnTokens); ok && FormatBigAmount(&body.Amount, AcmePrecisionPower) == "10.00000000" {
				seen[txn.Hash()] = true
			}
		}
		require.Len(t, seen, purchases, "each burn is on acc://ACME's main chain")
	})
}
