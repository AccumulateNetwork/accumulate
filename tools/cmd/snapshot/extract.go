// Copyright 2023 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package main

import (
	"os"

	"github.com/spf13/cobra"
	"gitlab.com/accumulatenetwork/accumulate/internal/database"
	"gitlab.com/accumulatenetwork/accumulate/internal/database/snapshot"
	"gitlab.com/accumulatenetwork/accumulate/pkg/database/keyvalue/badger"
	"gitlab.com/accumulatenetwork/accumulate/pkg/types/merkle"
	"gitlab.com/accumulatenetwork/accumulate/pkg/url"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

var extractCmd = &cobra.Command{
	Use:   "extract <database> <snapshot>",
	Short: "Extracts the non-system accounts from a snapshot for use as a Genesis snapshot of a new network",
	Args:  cobra.ExactArgs(2),
	Run:   extractSnapshot,
}

func init() { cmd.AddCommand(extractCmd) }

func extractSnapshot(_ *cobra.Command, args []string) {
	store1, err := badger.New(args[0])
	check(err)
	stx := store1.Begin(nil, false)
	defer stx.Discard()

	db1 := database.New(store1, nil)
	batch1 := db1.Begin(false)
	defer batch1.Discard()

	var accounts []*url.URL
	txnHashes := new(snapshot.HashSet)
	sigHashes := new(snapshot.HashSet)

	check(batch1.ForEachAccount(func(account *database.Account, hash [32]byte) error {
		acct, err := account.Main().Get()
		check(err)
		u := acct.GetUrl()

		// Skip system accounts
		if protocol.AcmeUrl().LocalTo(u) ||
			protocol.FaucetUrl.LocalTo(u) {
			return nil
		}
		if _, ok := protocol.ParsePartitionUrl(u); ok {
			return nil
		}

		accounts = append(accounts, u)
		pending, err := account.Pending().Get()
		checkf(err, "get %v pending", u)
		for _, txid := range pending {
			txnHashes.Add(txid.Hash())
		}

		err = txnHashes.CollectFromChain(account, account.MainChain())
		checkf(err, "get %v main chain", u)

		err = txnHashes.CollectFromChain(account, account.ScratchChain())
		checkf(err, "get %v scratch chain", u)

		err = sigHashes.CollectFromChain(account, account.SignatureChain())
		checkf(err, "get %v signature chain", u)

		return nil
	}))

	db2 := database.OpenInMemory(nil)
	batch2 := db2.Begin(true)
	defer batch2.Discard()
	for _, hash := range txnHashes.Hashes {
		hash := hash
		c, err := snapshot.CollectTransaction(batch1, hash)
		checkf(err, "collect txn %x", hash)
		err = c.Restore(new(snapshot.Header), batch2)
		checkf(err, "restore txn %x", hash)
		for _, c := range c.SignatureSets {
			for _, c := range c.Entries {
				sigHashes.Add(c.SignatureHash)
			}
		}
	}
	for _, hash := range sigHashes.Hashes {
		hash := hash
		c, err := snapshot.CollectSignature(batch1, hash)
		checkf(err, "collect sig %x", hash)
		err = c.Restore(new(snapshot.Header), batch2)
		checkf(err, "restore sig %x", hash)
	}
	for _, u := range accounts {
		acct, err := snapshot.CollectAccount(batch1.Account(u), true)
		checkf(err, "collect %v", u)
		err = acct.Restore(batch2)
		checkf(err, "restore %v", u)
		for _, c := range acct.Chains {
			if c.Type != merkle.ChainTypeTransaction {
				continue // Exclude index and anchor chains
			}
			c2, err := acct.RestoreChainHead(batch2, c)
			checkf(err, "restore %v %s chain", acct.Url, c.Name)
			err = c.RestoreMarkPointRange(c2.Inner(), 0, len(c.MarkPoints))
			checkf(err, "restore %v %s chain", acct.Url, c.Name)
		}
	}
	check(batch2.Commit())

	f, err := os.Create(args[1])
	check(err)
	defer f.Close()

	batch2 = db2.Begin(true)
	defer batch2.Discard()
	_, err = snapshot.Collect(batch2, new(snapshot.Header), f, snapshot.CollectOptions{})
	check(err)
}
