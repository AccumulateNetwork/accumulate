// Copyright 2023 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package light

import (
	"context"
	"encoding/json"
	"fmt"
	"math/big"
	"testing"

	"github.com/stretchr/testify/require"
	"gitlab.com/accumulatenetwork/accumulate/internal/database/smt/storage/memory"
	"gitlab.com/accumulatenetwork/accumulate/pkg/build"
	client "gitlab.com/accumulatenetwork/accumulate/pkg/client/api/v2"
	"gitlab.com/accumulatenetwork/accumulate/pkg/types/messaging"
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
		simulator.MemoryDatabase,
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
		err = c.IndexAnchors(ctx, u)
		require.NoError(t, err)
	}

	// Pull a random account
	err = c.PullTransactionsForAccount(ctx, alice.JoinPath("book", "1"), "main", "scratch")
	require.NoError(t, err)
	err = c.IndexAccountChains(ctx, alice.JoinPath("book", "1"))
	require.NoError(t, err)
}

func TestBridge(t *testing.T) {
	t.Skip("Manual")

	var err error
	c := new(Client)
	c.store = memory.New(nil)
	c.v2, err = client.New("mainnet")
	require.NoError(t, err)

	ctx := context.Background()
	acct := protocol.AccountUrl("bridge.acme", "1-ACME")
	err = c.PullAccount(ctx, acct)
	require.NoError(t, err)
	err = c.PullTransactionsForAccount(ctx, acct, "main")
	require.NoError(t, err)
	err = c.IndexAccountChains(ctx, acct)
	require.NoError(t, err)

	batch := c.OpenDB(false)
	defer batch.Discard()
	head, err := batch.Account(acct).MainChain().Head().Get()
	require.NoError(t, err)
	entries, err := batch.Account(acct).MainChain().Inner().GetRange(0, head.Count)
	require.NoError(t, err)

	var txns []*url.TxID
	deposits := map[[32]byte]*url.TxID{}
	for _, e := range entries {
		var msg *messaging.TransactionMessage
		err = batch.Message2(e).Main().GetAs(&msg)
		require.NoError(t, err)

		switch body := msg.Transaction.Body.(type) {
		case *protocol.SyntheticDepositTokens:
			txns = append(txns, body.Cause)
			deposits[body.Cause.Hash()] = msg.ID()

		case *protocol.SendTokens:
			break
			for _, to := range body.To {
				fmt.Printf("Sent %s tokens to %v (%v)\n",
					protocol.FormatBigAmount(&to.Amount, protocol.AcmePrecisionPower),
					to.Url,
					msg.ID())
			}
		}
	}

	err = c.PullTransactions(ctx, txns...)
	require.NoError(t, err)

	var outputs []*protocol.TokenRecipient
	index := map[[32]byte]int{}
	addOutput := func(u *url.URL, amount *big.Int) {
		// Find the existing output or create a new one
		var out *protocol.TokenRecipient
		i, ok := index[u.AccountID32()]
		if ok {
			out = outputs[i]
		} else {
			index[u.AccountID32()] = len(outputs)
			out = &protocol.TokenRecipient{Url: u}
			outputs = append(outputs, out)
		}

		out.Amount.Add(&out.Amount, amount)
	}

	for _, id := range txns {
		var msg *messaging.TransactionMessage
		err = batch.Message(id.Hash()).Main().GetAs(&msg)
		require.NoError(t, err)

		if msg.Transaction.Header.Memo != "" {
			continue
		}

		send, ok := msg.Transaction.Body.(*protocol.SendTokens)
		if !ok {
			continue
		}
		require.Len(t, send.To, 1) // Need to update the test if there are any multi-output sends

		fmt.Printf("Received %s tokens from %v without memo (%v -> %v)\n",
			protocol.FormatBigAmount(&send.To[0].Amount, protocol.AcmePrecisionPower),
			msg.Transaction.Header.Principal,
			id, deposits[id.Hash()])

		addOutput(msg.Transaction.Header.Principal, &send.To[0].Amount)
	}

	b, err := json.MarshalIndent(outputs, "", "  ")
	require.NoError(t, err)
	fmt.Printf("%s\n", b)
}
