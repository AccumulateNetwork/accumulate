// Copyright 2024 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package indexing_test

import (
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"gitlab.com/accumulatenetwork/accumulate/internal/database"
	"gitlab.com/accumulatenetwork/accumulate/pkg/build"
	"gitlab.com/accumulatenetwork/accumulate/pkg/url"
	. "gitlab.com/accumulatenetwork/accumulate/protocol"
	. "gitlab.com/accumulatenetwork/accumulate/test/harness"
	. "gitlab.com/accumulatenetwork/accumulate/test/helpers"
	"gitlab.com/accumulatenetwork/accumulate/test/simulator"
)

func TestNetworkHistory(t *testing.T) {
	// Setup
	alice := build.
		Identity("alice").Create("book").
		Tokens("tokens").Create("ACME").Add(1e9).Identity().
		Book("book").Page(1).Create().AddCredits(1e9).Book().Identity()
	aliceKey := alice.Book("book").Page(1).
		GenerateKey(SignatureTypeED25519)

	sim := NewSim(t,
		simulator.SimpleNetwork(t.Name(), 1, 1),
		simulator.Genesis(GenesisTime).With(alice),
	)

	// Generate history
	var st *TransactionStatus
	var timestamp uint64
	start := time.Now()
	for i := range 500 {
		st = sim.BuildAndSubmitTxnSuccessfully(
			build.Transaction().For(alice, "book", "1").
				BurnCredits(1).
				SignWith(alice, "book", "1").Version(1).Timestamp(&timestamp).PrivateKey(aliceKey))

		sim.Step()
		if (i+1)%20 == 0 {
			fmt.Printf("%d (%v)\n", i+1, time.Since(start)/time.Duration(i+1))
		}
	}
	sim.StepUntil(
		Txn(st.TxID).Completes())

	// Check the root index
	View(t, sim.Database("BVN0"), func(batch *database.Batch) {
		a := batch.Account(url.MustParse("bvn-bvn0.acme/ledger"))
		h, err := a.RootChain().Index().Head().Get()
		require.NoError(t, err)
		t.Log(h.Count)
	})
}
