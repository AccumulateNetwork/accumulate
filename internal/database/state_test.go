// Copyright 2022 The Accumulate Authors
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

	"github.com/stretchr/testify/require"
	"gitlab.com/accumulatenetwork/accumulate/internal/database"
	"gitlab.com/accumulatenetwork/accumulate/internal/database/snapshot"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
	simulator "gitlab.com/accumulatenetwork/accumulate/test/simulator/compat"
	acctesting "gitlab.com/accumulatenetwork/accumulate/test/testing"
)

func init() { acctesting.EnableDebugFeatures() }

var delivered = (*protocol.TransactionStatus).Delivered

func TestState(t *testing.T) {
	// Create some state
	sim := simulator.New(t, 1)
	sim.InitFromGenesis()
	alice := acctesting.GenerateTmKey(t.Name(), "Alice")
	aliceUrl := acctesting.AcmeLiteAddressTmPriv(alice)
	faucet := protocol.Faucet.Signer()
	env := acctesting.NewTransaction().
		WithPrincipal(protocol.FaucetUrl).
		WithTimestamp(faucet.Timestamp()).
		WithBody(&protocol.AcmeFaucet{Url: aliceUrl}).
		Faucet()
	sim.MustSubmitAndExecuteBlock(env)
	sim.WaitForTransactionFlow(delivered, env.Transaction[0].GetHash())

	sim.ExecuteBlocks(10)

	// Save to a file
	f, err := os.Create(filepath.Join(t.TempDir(), "state.bpt"))
	require.NoError(t, err)
	defer f.Close()

	bvn := sim.PartitionFor(aliceUrl)
	var blockHash, bptRoot []byte
	_ = bvn.Database.View(func(b *database.Batch) error {
		blockHash, err = b.GetMinorRootChainAnchor(&bvn.Executor.Describe)
		require.NoError(t, err)
		require.NoError(t, snapshot.FullCollect(b, f, bvn.Executor.Describe.PartitionUrl(), nil, false))
		bptRoot = b.BptRoot()
		return nil
	})

	_, err = f.Seek(0, io.SeekStart)
	require.NoError(t, err)

	// Load the file into a new database
	db := database.OpenInMemory(nil)
	require.NoError(t, db.Update(func(b *database.Batch) error {
		return snapshot.FullRestore(b, f, nil, &bvn.Executor.Describe)
	}))
	require.NoError(t, db.View(func(b *database.Batch) error {
		// Does it match?
		blockHash2, err := b.GetMinorRootChainAnchor(&bvn.Executor.Describe)
		require.NoError(t, err)
		require.Equal(t, blockHash, blockHash2)
		require.Equal(t, bptRoot, b.BptRoot())
		return nil
	}))

}
