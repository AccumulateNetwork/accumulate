// Copyright 2025 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package database_test

import (
	"io"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"gitlab.com/accumulatenetwork/accumulate/internal/core"
	"gitlab.com/accumulatenetwork/accumulate/internal/database"
	"gitlab.com/accumulatenetwork/accumulate/internal/database/snapshot"
	"gitlab.com/accumulatenetwork/accumulate/pkg/build"
	"gitlab.com/accumulatenetwork/accumulate/pkg/url"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
	. "gitlab.com/accumulatenetwork/accumulate/test/helpers"
	simulator "gitlab.com/accumulatenetwork/accumulate/test/simulator/compat"
	acctesting "gitlab.com/accumulatenetwork/accumulate/test/testing"
)

func init() { acctesting.EnableDebugFeatures() }

var delivered = (*protocol.TransactionStatus).Delivered

func TestState(t *testing.T) {
	// Create some state
	sim := simulator.New(t, 1)
	sim.InitFromGenesisWith(&core.GlobalValues{ExecutorVersion: protocol.ExecutorVersionV1})
	alice := acctesting.GenerateTmKey(t.Name(), "Alice")
	aliceUrl := acctesting.AcmeLiteAddressTmPriv(alice)
	env :=
		MustBuild(t, build.Transaction().
			For(protocol.FaucetUrl).
			Body(&protocol.AcmeFaucet{Url: aliceUrl}).
			SignWith(protocol.FaucetUrl).Version(1).Timestamp(time.Now().UnixNano()).Signer(protocol.Faucet.Signer()))
	sim.MustSubmitAndExecuteBlock(env)
	sim.WaitForTransactionFlow(delivered, env.Transaction[0].GetHash())

	sim.ExecuteBlocks(10)

	// Save to a file
	f, err := os.Create(filepath.Join(t.TempDir(), "state.bpt"))
	require.NoError(t, err)
	defer f.Close()

	bvn := sim.PartitionFor(aliceUrl)
	var blockHash []byte
	var bptRoot [32]byte
	_ = bvn.Database.View(func(b *database.Batch) error {
		blockHash, err = b.GetMinorRootChainAnchor(&bvn.Executor.Describe)
		require.NoError(t, err)
		require.NoError(t, snapshot.FullCollect(b, f, bvn.Executor.Describe.PartitionUrl(), nil, false))
		bptRoot, err = b.BPT().GetRootHash()
		require.NoError(t, err)
		return nil
	})

	_, err = f.Seek(0, io.SeekStart)
	require.NoError(t, err)

	hashes := acctesting.VisitorObserver{}
	require.NoError(t, snapshot.Visit(f, hashes))
	_, err = f.Seek(0, io.SeekStart)
	require.NoError(t, err)

	// Load the file into a new database
	db := database.OpenInMemory(nil)
	db.SetObserver(hashes)
	require.NoError(t, db.Update(func(b *database.Batch) error {
		return snapshot.FullRestore(b, f, nil, bvn.Executor.Describe.PartitionUrl())
	}))
	require.NoError(t, db.View(func(b *database.Batch) error {
		// Does it match?
		blockHash2, err := b.GetMinorRootChainAnchor(&bvn.Executor.Describe)
		require.NoError(t, err)
		bptRoot2, err := b.BPT().GetRootHash()
		require.NoError(t, err)
		require.Equal(t, blockHash, blockHash2)
		require.Equal(t, bptRoot, bptRoot2)
		return nil
	}))

}

func TestVersion(t *testing.T) {
	logger := acctesting.NewTestLogger(t)
	db := database.OpenInMemory(logger)
	db.SetObserver(acctesting.NullObserver{})

	foo := protocol.AccountUrl("foo")
	get := func(batch *database.Batch) (a *protocol.UnknownSigner) {
		require.NoError(t, batch.Account(foo).Main().GetAs(&a))
		return a
	}

	set := func(batch *database.Batch, a *protocol.UnknownSigner, version uint64) {
		a.Version = version
		require.NoError(t, batch.Account(foo).Main().Put(a))
	}

	root := db.Begin(true)
	set(root, &protocol.UnknownSigner{Url: foo}, 0)

	// Safe
	batch := root.Begin(true)
	set(batch, &protocol.UnknownSigner{Url: foo}, 1)
	a := get(batch)
	set(batch, a, 2)
	require.NoError(t, batch.Commit())

	// Safe
	batch = root.Begin(true)
	set(batch, get(batch), 3)
	sub := batch.Begin(true)
	set(sub, get(sub), 4)
	require.NoError(t, sub.Commit())
	require.NoError(t, batch.Commit())

	// Unsafe
	batch = root.Begin(true)
	sub = batch.Begin(true)
	a = get(batch)
	b := get(sub)
	set(batch, a, 5)
	set(sub, b, 6)
	require.NoError(t, sub.Commit())
	require.NoError(t, batch.Commit())
}

func TestNonLedgerEvents(t *testing.T) {
	db := database.OpenInMemory(nil)
	db.SetObserver(acctesting.NullObserver{})

	// Try to add events to a random account
	batch := db.Begin(true)
	defer batch.Discard()
	foo := batch.Account(url.MustParse("foo"))
	require.NoError(t, foo.Events().Minor().Blocks().Add(1))

	err := foo.Commit()
	require.EqualError(t, err, "acc://foo is not allowed to have events/blocks")
}
