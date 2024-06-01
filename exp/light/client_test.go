// Copyright 2024 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package light

import (
	"context"
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"
	"gitlab.com/accumulatenetwork/accumulate/pkg/build"
	"gitlab.com/accumulatenetwork/accumulate/pkg/database/keyvalue/memory"
	"gitlab.com/accumulatenetwork/accumulate/pkg/types/network"
	"gitlab.com/accumulatenetwork/accumulate/pkg/url"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
	. "gitlab.com/accumulatenetwork/accumulate/test/harness"
	. "gitlab.com/accumulatenetwork/accumulate/test/helpers"
	"gitlab.com/accumulatenetwork/accumulate/test/simulator"
	acctesting "gitlab.com/accumulatenetwork/accumulate/test/testing"
)

func init() {
	acctesting.EnableDebugFeatures()
}

func TestClient(t *testing.T) {
	alice := protocol.AccountUrl("alice")
	aliceKey := acctesting.GenerateKey(alice)

	// Initialize
	sim := NewSim(t,
		simulator.SimpleNetwork(t.Name(), 3, 3),
		simulator.Genesis(GenesisTime),
	)

	MakeIdentity(t, sim.DatabaseFor(alice), alice, aliceKey[32:])
	CreditCredits(t, sim.DatabaseFor(alice), alice.JoinPath("book", "1"), 100*protocol.CreditPrecision+1)
	MakeKeyPage(t, sim.DatabaseFor(alice), alice.JoinPath("book"))

	// Execute
	st := sim.BuildAndSubmitTxnSuccessfully(
		build.Transaction().For(alice, "book", "1").
			TransferCredits(100).To(alice.JoinPath("book", "2")).
			SignWith(alice, "book", "1").Version(1).Timestamp(1).PrivateKey(aliceKey))

	sim.StepUntil(
		Txn(st.TxID).Succeeds())

	var err error
	c := new(Client)
	c.store = memory.New(nil)
	c.v2 = sim.S.ClientV2(protocol.Directory)
	c.query.Querier = sim.Query()

	// Pull the globals accounts
	fmt.Println("Load globals")
	ctx := context.Background()
	err = c.PullAccount(ctx, protocol.DnUrl().JoinPath(protocol.Ledger))
	require.NoError(t, err)
	err = c.PullAccount(ctx, protocol.DnUrl().JoinPath(protocol.Globals))
	require.NoError(t, err)
	err = c.PullAccount(ctx, protocol.DnUrl().JoinPath(protocol.Network))
	require.NoError(t, err)
	err = c.PullAccount(ctx, protocol.DnUrl().JoinPath(protocol.Oracle))
	require.NoError(t, err)
	err = c.PullAccount(ctx, protocol.DnUrl().JoinPath(protocol.Routing))
	require.NoError(t, err)

	// Load the globals
	g := new(network.GlobalValues)
	batch := c.OpenDB(false)
	defer batch.Discard()
	err = g.Load(protocol.DnUrl(), func(accountUrl *url.URL, target interface{}) error {
		return batch.Account(accountUrl).Main().GetAs(target)
	})
	require.NoError(t, err)

	for _, p := range g.Network.Partitions {
		fmt.Printf("Load %s ledger\n", p.ID)

		// Pull the partition's system ledger
		u := protocol.PartitionUrl(p.ID)
		err = c.PullAccount(ctx, u.JoinPath(protocol.Ledger))
		require.NoError(t, err)
		err = c.IndexAccountChains(ctx, u.JoinPath(protocol.Ledger))
		require.NoError(t, err)

		// Pull the partition's anchor ledger
		err = c.PullAccount(ctx, u.JoinPath(protocol.AnchorPool))
		require.NoError(t, err)
		err = c.IndexAccountChains(ctx, u.JoinPath(protocol.AnchorPool))
		require.NoError(t, err)

		// Pull the partition's anchors
		err = c.PullTransactionsForAccount(ctx, u.JoinPath(protocol.AnchorPool), "anchor-sequence")
		require.NoError(t, err)

		// Index the anchors
		err = c.IndexProducedAnchors(ctx, u)
		require.NoError(t, err)
	}

	// Pull a random account
	err = c.PullTransactionsForAccount(ctx, alice.JoinPath("book", "1"), "main", "scratch")
	require.NoError(t, err)
	err = c.IndexAccountChains(ctx, alice.JoinPath("book", "1"))
	require.NoError(t, err)
}
