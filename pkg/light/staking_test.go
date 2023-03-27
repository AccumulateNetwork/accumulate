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
	"testing"

	"github.com/stretchr/testify/require"
	"gitlab.com/accumulatenetwork/accumulate/internal/core"
	"gitlab.com/accumulatenetwork/accumulate/internal/database/smt/storage/badger"
	"gitlab.com/accumulatenetwork/accumulate/internal/node/config"
	client "gitlab.com/accumulatenetwork/accumulate/pkg/client/api/v2"
	"gitlab.com/accumulatenetwork/accumulate/pkg/errors"
	"gitlab.com/accumulatenetwork/accumulate/pkg/types/messaging"
	"gitlab.com/accumulatenetwork/accumulate/pkg/url"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

func TestStaking(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	_ = ctx

	db, err := badger.New("/tmp/staking.db", nil)
	require.NoError(t, err)
	defer db.Close()

	c := new(Client)
	c.v2, err = client.New("mainnet")
	require.NoError(t, err)
	c.store = db

	pullForStaking(t, c, ctx)
	runStaking(t, c)
}

func runStaking(t *testing.T, c *Client) {
	batch := c.OpenDB(false)
	defer batch.Discard()

	rootIndex, err := batch.Index().Account(protocol.DnUrl().JoinPath(protocol.Ledger)).Chain("root").Index().Get()
	require.NoError(t, err)
	majorIndex, err := batch.Index().Account(protocol.DnUrl().JoinPath(protocol.AnchorPool)).Chain("major-block").Index().Get()
	require.NoError(t, err)

	u := protocol.AccountUrl("reesor.acme", "stake")
	head, err := batch.Account(u).MainChain().Head().Get()
	require.NoError(t, err)
	entries, err := batch.Account(u).MainChain().Inner().GetRange(0, head.Count)
	require.NoError(t, err)

	for _, e := range entries {
		hash := *(*[32]byte)(e)
		var msg *messaging.TransactionMessage
		err = batch.Message(hash).Main().GetAs(&msg)
		require.NoError(t, err)
		executed, err := batch.Index().Transaction(hash).Executed().Get()
		require.NoError(t, err)

		i, _, ok := FindEntry(rootIndex, ByIndexBlock(executed.DirectoryBlock))
		require.True(t, ok)

		var extra string
		_, major, ok := FindEntry(majorIndex, ByIndexRootIndexIndex(uint64(i)))
		if ok {
			extra = fmt.Sprintf(", major: %d", major.BlockIndex)
		}

		fmt.Printf("Executed %v on %v (local: %d, dir: %d%s)\n", msg.Transaction.Body.Type(), executed.LocalTime, executed.LocalBlock, executed.DirectoryBlock, extra)
	}
}

func pullForStaking(t *testing.T, c *Client, ctx context.Context) {
	// Pull the globals accounts
	fmt.Println("Load globals")
	err := c.PullAccount(ctx, protocol.DnUrl().JoinPath(protocol.Ledger))
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
	g := new(core.GlobalValues)
	batch := c.OpenDB(false)
	defer batch.Discard()
	err = g.Load(config.NetworkUrl{URL: protocol.DnUrl()}, func(accountUrl *url.URL, target interface{}) error {
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

	fmt.Print("\n\n\n")

	// Pull staking registered list
	fmt.Println("Load staking")
	registered := protocol.AccountUrl("staking.acme", "registered")
	err = c.PullAccount(ctx, registered)
	require.NoError(t, err)
	err = c.PullTransactionsForAccount(ctx, registered, "main", "scratch")
	require.NoError(t, err)

	// Scan the registered list
	head, err := batch.Account(registered).MainChain().Head().Get()
	require.NoError(t, err)
	entries, err := batch.Account(registered).MainChain().Inner().GetRange(0, head.Count)
	require.NoError(t, err)
	seen := map[[32]byte]bool{}
	var accounts []*url.URL
	for i := len(entries) - 1; i >= 300; i-- {
		var msg *messaging.TransactionMessage
		err = batch.Message2(entries[i]).Main().GetAs(&msg)
		require.NoError(t, err)
		body, ok := msg.Transaction.Body.(*protocol.WriteData)
		if !ok {
			continue
		}

		var entry struct{ Stake *url.URL }
		err = json.Unmarshal(body.Entry.GetData()[0], &entry)
		require.NoError(t, err)

		if seen[entry.Stake.AccountID32()] {
			continue
		}
		seen[entry.Stake.AccountID32()] = true

		// Pull and index the staking account
		err := c.PullAccount(ctx, entry.Stake)
		if errors.Is(err, errors.NotFound) {
			continue // Skip bad accounts
		}
		accounts = append(accounts, entry.Stake)

		require.NoError(t, err)
		err = c.PullTransactionsForAccount(ctx, entry.Stake, "main", "scratch")
		require.NoError(t, err)
		err = c.IndexAccountChains(ctx, entry.Stake)
		require.NoError(t, err)
	}

	err = c.IndexAccountTransactions(ctx, accounts...)
	require.NoError(t, err)
}
