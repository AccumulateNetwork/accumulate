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
		simulator.Genesis(GenesisTime).With(alice).WithVersion(ExecutorVersionV2Vandenberg),
		simulator.BadgerDatabaseFromDirectory("/tmp/accumulate-test-network-history", func(err error) { require.NoError(t, err) }),
	)
	before := getHeight(t, sim)

	// Generate history
	var st *TransactionStatus
	start := time.Now()
	var last time.Duration
	for i := 0; ; i++ {
		if i%8 == 0 {
			st = sim.BuildAndSubmitTxnSuccessfully(
				build.Transaction().For(alice, "book", "1").
					BurnCredits(1).
					SignWith(alice, "book", "1").Version(1).Timestamp(time.Now().UnixMicro()).PrivateKey(aliceKey))
		}

		sim.Step()

		d := time.Since(start)
		if d-last > 2*time.Second {
			last = d
			h := getHeight(t, sim)
			fmt.Printf("%d (%d blocks, %v per)\n", h, i+1, d/time.Duration(i+1))
			if h > 10500 {
				break
			}
		}
	}
	sim.StepUntil(
		Txn(st.TxID).Completes())

	// Check the root index
	h := getHeight(t, sim)
	fmt.Printf("%d (%v)\n", h, time.Since(start)/time.Duration(h-before))
}

func getHeight(t testing.TB, sim *Sim) uint64 {
	var height uint64
	View(t, sim.Database("BVN0"), func(batch *database.Batch) {
		a := batch.Account(url.MustParse("bvn-bvn0.acme/ledger"))
		h, err := a.RootChain().Index().Head().Get()
		require.NoError(t, err)
		height = uint64(h.Count)
	})
	return height
}
