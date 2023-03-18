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
	"gitlab.com/accumulatenetwork/accumulate/internal/database/smt/storage"
	"gitlab.com/accumulatenetwork/accumulate/internal/database/smt/storage/memory"
	"gitlab.com/accumulatenetwork/accumulate/internal/node/config"
	accumulated "gitlab.com/accumulatenetwork/accumulate/internal/node/daemon"
	"gitlab.com/accumulatenetwork/accumulate/pkg/build"
	. "gitlab.com/accumulatenetwork/accumulate/protocol"
	. "gitlab.com/accumulatenetwork/accumulate/test/harness"
	. "gitlab.com/accumulatenetwork/accumulate/test/helpers"
	"gitlab.com/accumulatenetwork/accumulate/test/simulator"
	acctesting "gitlab.com/accumulatenetwork/accumulate/test/testing"
)

func init() {
	acctesting.EnableDebugFeatures()
}

func TestSimulator(t *testing.T) {
	g := new(core.GlobalValues)
	g.Globals = new(NetworkGlobals)
	g.Globals.OperatorAcceptThreshold.Set(1, 100) // Use a small number so M = 1
	g.ExecutorVersion = ExecutorVersionLatest

	net := simulator.SimpleNetwork(t.Name(), 1, 1)
	net.Bsn = &accumulated.BvnInit{
		Id: "BSN",
		Nodes: []*accumulated.NodeInit{{
			BsnnType:   config.NodeTypeValidator,
			PrivValKey: acctesting.GenerateKey(t.Name(), "BSN", 0, "val"),
			BsnNodeKey: acctesting.GenerateKey(t.Name(), "BSN", 0, "node"),
		}},
	}

	var bsnStore *memory.DB
	openDb := func(partition string, node int, logger log.Logger) storage.KeyValueStore {
		if partition == net.Bsn.Id && node == 0 {
			bsnStore = memory.New(logger)
			return bsnStore
		}
		return memory.New(logger)
	}

	// Initialize
	sim := NewSim(t,
		openDb,
		net,
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

	sim.StepN(10)

	// Verify
	account := GetAccount[*LiteTokenAccount](t, sim.DatabaseFor(lite), lite)
	require.Equal(t, 123, int(account.Balance.Int64()))

	batch := bsn.NewChangeSet(bsnStore, nil)
	defer batch.Discard()
	account2 := GetAccount[*LiteTokenAccount](t, batch.Partition("BVN0"), lite)
	require.True(t, account.Equal(account2))
}
