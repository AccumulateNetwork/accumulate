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
	"gitlab.com/accumulatenetwork/accumulate/pkg/types/record"
	"gitlab.com/accumulatenetwork/accumulate/pkg/url"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
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

	badger := simulator.BadgerDbOpener("/tmp/accumulate-test-network-history", func(err error) { require.NoError(t, err) })
	_ = badger

	sim := NewSim(t,
		simulator.SimpleNetwork(t.Name(), 1, 1),
		simulator.Genesis(GenesisTime).With(alice).WithVersion(ExecutorVersionV2Vandenberg),
		simulator.OverlayDatabase(simulator.MemoryDbOpener, badger),
	)

	// Generate history
	var st *TransactionStatus
	start := time.Now()
	var last time.Duration
	for i := 0; getHeight(t, sim) < 12000; i++ {
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
			fmt.Printf("%d (%d blocks, %v per)\n", getHeight(t, sim), i+1, d/time.Duration(i+1))
		}
	}
	if st != nil {
		sim.StepUntil(
			Txn(st.TxID).Completes())
	}

	start = time.Now()
	Update(t, sim.Database("BVN0"), func(batch *database.Batch) {
		a := batch.Account(url.MustParse("bvn-bvn0.acme/ledger"))
		c := a.RootChain().Index()
		h, err := c.Head().Get()
		require.NoError(t, err)
		for i := 0; i < int(h.Count); i += 256 {
			entries, err := c.Inner().Entries(int64(i), int64(i+256))
			require.NoError(t, err)

			for _, h := range entries {
				entry := new(protocol.IndexEntry)
				require.NoError(t, entry.UnmarshalBinary(h))
				require.NoError(t, a.BlockLedger().Append(record.NewKey(entry.BlockIndex), &database.BlockLedger{}))
			}
		}
	})
	fmt.Println("Allocating blocks took", time.Since(start))
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
