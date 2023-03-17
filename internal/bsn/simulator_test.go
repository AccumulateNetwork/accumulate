// Copyright 2023 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package bsn_test

import (
	"bytes"
	"testing"

	"github.com/stretchr/testify/require"
	"gitlab.com/accumulatenetwork/accumulate/internal/core"
	"gitlab.com/accumulatenetwork/accumulate/internal/database"
	"gitlab.com/accumulatenetwork/accumulate/internal/database/snapshot"
	"gitlab.com/accumulatenetwork/accumulate/internal/node/config"
	accumulated "gitlab.com/accumulatenetwork/accumulate/internal/node/daemon"
	ioutil2 "gitlab.com/accumulatenetwork/accumulate/internal/util/io"
	"gitlab.com/accumulatenetwork/accumulate/pkg/build"
	. "gitlab.com/accumulatenetwork/accumulate/protocol"
	. "gitlab.com/accumulatenetwork/accumulate/test/harness"
	"gitlab.com/accumulatenetwork/accumulate/test/helpers"
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

	// Use the simulator to create genesis documents
	net := simulator.SimpleNetwork(t.Name(), 1, 1)
	sim := NewSim(t,
		simulator.MemoryDatabase,
		net,
		simulator.GenesisWith(GenesisTime, g),
	)

	sim.StepN(100)

	// Capture the snapshot now as the genesis snapshot
	genesis := map[string][]byte{}
	for _, part := range sim.Partitions() {
		helpers.View(t, sim.Database(part.ID), func(batch *database.Batch) {
			buf := new(ioutil2.Buffer)
			_, err := snapshot.Collect(batch, new(snapshot.Header), buf, snapshot.CollectOptions{
				PreserveAccountHistory: func(*database.Account) (bool, error) { return true, nil },
			})
			require.NoError(t, err)
			genesis[part.ID] = buf.Bytes()
		})
	}

	// Add the BSN
	net.Bsn = &accumulated.BvnInit{
		Id: "BSN",
		Nodes: []*accumulated.NodeInit{{
			BsnnType:   config.NodeTypeValidator,
			PrivValKey: acctesting.GenerateKey(t.Name(), "BSN", 0, "val"),
			BsnNodeKey: acctesting.GenerateKey(t.Name(), "BSN", 0, "node"),
		}},
	}

	// Build the BSN's snapshot
	header := new(snapshot.Header)
	header.Height = 1
	header.Timestamp = GenesisTime
	header.PartitionSnapshotIDs = []string{Directory}
	for _, b := range net.Bvns {
		header.PartitionSnapshotIDs = append(header.PartitionSnapshotIDs, b.Id)
	}
	{
		buf := new(ioutil2.Buffer)
		snap, err := snapshot.Create(buf, header)
		require.NoError(t, err)

		for _, id := range header.PartitionSnapshotIDs {
			w, err := snap.Open(snapshot.SectionTypeSnapshot)
			require.NoError(t, err)
			_, err = bytes.NewBuffer(genesis[id]).WriteTo(w)
			require.NoError(t, err)
			err = w.Close()
			require.NoError(t, err)
		}
		genesis[net.Bsn.Id] = buf.Bytes()
	}

	// Initialize
	sim = NewSim(t,
		simulator.MemoryDatabase,
		net,
		// simulator.SnapshotMap(genesis),
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

	// Verify
	account := GetAccount[*LiteTokenAccount](t, sim.DatabaseFor(lite), lite)
	require.Equal(t, 123, int(account.Balance.Int64()))
}
