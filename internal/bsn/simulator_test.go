// Copyright 2023 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package bsn_test

import (
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/tendermint/tendermint/libs/log"
	"gitlab.com/accumulatenetwork/accumulate/internal/bsn"
	"gitlab.com/accumulatenetwork/accumulate/internal/core"
	"gitlab.com/accumulatenetwork/accumulate/internal/node/config"
	accumulated "gitlab.com/accumulatenetwork/accumulate/internal/node/daemon"
	"gitlab.com/accumulatenetwork/accumulate/pkg/build"
	"gitlab.com/accumulatenetwork/accumulate/pkg/database/keyvalue"
	"gitlab.com/accumulatenetwork/accumulate/pkg/database/keyvalue/memory"
	. "gitlab.com/accumulatenetwork/accumulate/protocol"
	. "gitlab.com/accumulatenetwork/accumulate/test/harness"
	. "gitlab.com/accumulatenetwork/accumulate/test/helpers"
	"gitlab.com/accumulatenetwork/accumulate/test/simulator"
	acctesting "gitlab.com/accumulatenetwork/accumulate/test/testing"
)

func init() {
	acctesting.EnableDebugFeatures()
}

func simpleNetwork(name string, bvnCount, nodeCount int) simulator.Option {
	net := simulator.NewSimpleNetwork(name, bvnCount, nodeCount)
	net.Bsn = &accumulated.BvnInit{
		Id: "BSN",
		Nodes: []*accumulated.NodeInit{{
			BsnnType:   config.NodeTypeValidator,
			PrivValKey: acctesting.GenerateKey(name, "BSN", 0, "val"),
			BsnNodeKey: acctesting.GenerateKey(name, "BSN", 0, "node"),
		}},
	}
	return simulator.WithNetwork(net)
}

func captureBsnStore(db **memory.Database) simulator.Option {
	return simulator.WithDatabase(func(partition *PartitionInfo, node int, logger log.Logger) keyvalue.Beginner {
		if partition.Type == PartitionTypeBlockSummary && node == 0 {
			*db = memory.New(nil)
			return *db
		}
		return memory.New(nil)
	})
}

func TestSimulator(t *testing.T) {
	g := new(core.GlobalValues)
	g.Globals = new(NetworkGlobals)
	g.Globals.OperatorAcceptThreshold.Set(1, 100) // Use a small number so M = 1
	g.ExecutorVersion = ExecutorVersionLatest

	// Initialize
	var bsnStore *memory.Database
	sim := NewSim(t,
		captureBsnStore(&bsnStore),
		simpleNetwork(t.Name(), 1, 1),
		simulator.GenesisWith(GenesisTime, g),
	)

	// Create an LTA
	liteKey := acctesting.GenerateKey("lite")
	lite := acctesting.AcmeLiteAddressStdPriv(liteKey)

	// Execute
	st := sim.BuildAndSubmitTxnSuccessfully(
		build.Transaction().For(ACME).
			IssueTokens(123, 0).To(lite).
			SignWith(DnUrl(), Operators, "1").Version(1).Timestamp(1).Signer(sim.SignWithNode(Directory, 0)))

	sim.StepUntil(
		Txn(st.TxID).Succeeds(),
		Txn(st.TxID).Produced().Succeeds())

	// Verify the transaction
	account := GetAccount[*LiteTokenAccount](t, sim.DatabaseFor(lite), lite)
	require.Equal(t, 123, int(account.Balance.Int64()))

	// Wait for the BSN to sync
	sim.StepN(10)

	// Verify the BSN's version is the same
	batch := bsn.NewChangeSet(bsnStore, nil)
	defer batch.Discard()
	account2 := GetAccount[*LiteTokenAccount](t, batch.Partition("BVN0"), lite)
	require.True(t, account.Equal(account2))
}
