// Copyright 2025 The Accumulate Authors
// 
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package indexing_test

import (
	"errors"
	"flag"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
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

var buildNetworkHistoryDb = flag.Bool("build-network-history-db", false, "Build the database for testing network history")

func TestNetworkHistory(t *testing.T) {
	dir := filepath.Join(os.TempDir(), "accumulate-"+t.Name()+".badger")
	if !*buildNetworkHistoryDb {
		if ent, err := os.ReadDir(dir); len(ent) == 0 {
			if !errors.Is(err, fs.ErrNotExist) {
				require.NoError(t, err)
			}
			t.Skip("Run with -build-network-history-db to build the database")
		}
	}

	// Setup
	alice := build.
		Identity("alice").Create("book").
		Tokens("tokens").Create("ACME").Add(1e9).Identity().
		Book("book").Page(1).Create().AddCredits(1e9).Book().Identity()
	aliceKey := alice.Book("book").Page(1).
		GenerateKey(SignatureTypeED25519)

	badger := simulator.BadgerDbOpener(dir, func(err error) { require.NoError(t, err) })
	simOpts := []simulator.Option{
		simulator.SimpleNetwork(t.Name(), 1, 1),
		simulator.Genesis(GenesisTime).With(alice).WithVersion(ExecutorVersionV2Vandenberg),
	}

	// Generate history
	if *buildNetworkHistoryDb {
		sim := NewSim(t, append(simOpts,
			simulator.WithDatabase(badger),
		)...)

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
	}

	sim := NewSim(t, append(simOpts,
		simulator.OverlayDatabase(simulator.MemoryDbOpener, badger),
	)...)

	// Update to Jiuquan
	st := sim.SubmitTxnSuccessfully(MustBuild(t,
		build.Transaction().For(DnUrl()).
			ActivateProtocolVersion(ExecutorVersionV2Jiuquan).
			SignWith(DnUrl(), Operators, "1").Version(1).Timestamp(1).Signer(sim.SignWithNode(Directory, 0))))

	sim.StepUntil(
		Txn(st.TxID).Succeeds(),
		VersionIs(ExecutorVersionV2Jiuquan))

	View(t, sim.Database(Directory), func(batch *database.Batch) {
		a := batch.Account(DnUrl().JoinPath(Ledger))
		k, _, err := a.BlockLedger().Last()
		require.NoError(t, err)
		require.Equal(t, sim.S.BlockIndex(Directory), k.Get(0))
	})
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
