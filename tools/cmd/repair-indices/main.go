// Copyright 2025 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package main

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
	"gitlab.com/accumulatenetwork/accumulate/internal/database"
	"gitlab.com/accumulatenetwork/accumulate/internal/database/indexing"
	"gitlab.com/accumulatenetwork/accumulate/internal/node/config"
	accumulated "gitlab.com/accumulatenetwork/accumulate/internal/node/daemon"
	"gitlab.com/accumulatenetwork/accumulate/pkg/errors"
	"gitlab.com/accumulatenetwork/accumulate/pkg/types/messaging"
	"gitlab.com/accumulatenetwork/accumulate/pkg/url"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

func main() {
	_ = cmd.Execute()
}

var cmd = &cobra.Command{
	Use:   "repair-indices [node]",
	Short: "Repair Accumulate's indices",
	Args:  cobra.ExactArgs(1),
	Run:   run,
}

func run(_ *cobra.Command, args []string) {
	daemon, err := accumulated.Load(args[0], nil)
	check(err)

	db, err := database.Open(daemon.Config, daemon.Logger)
	check(err)

	err = rebuildIndices(db, config.NetworkUrl{URL: protocol.PartitionUrl(daemon.Config.Accumulate.PartitionId)})
	check(err)
}

func rebuildIndices(db database.Beginner, partition config.NetworkUrl) error {
	accounts, err := collect(db, partition)
	if err != nil {
		return errors.UnknownError.Wrap(err)
	}

	batch := db.Begin(true)
	defer batch.Discard()

	for _, a := range accounts {
		fmt.Printf("Rebuilding index for %v (%d entries)\n", a.Account, len(a.Entries))
		r := batch.Account(a.Account)
		var entryHashes [][32]byte
		for _, e := range a.Entries {
			entryHashes = append(entryHashes, e.Entry)
			err = r.Data().Transaction(e.Entry).Put(e.Txn)
			if err != nil {
				return errors.UnknownError.Wrap(err)
			}
		}

		err = r.Data().Entry().Overwrite(entryHashes)
		if err != nil {
			return errors.UnknownError.Wrap(err)
		}
	}

	err = batch.Commit()
	return errors.UnknownError.Wrap(err)
}

type Data struct {
	Account *url.URL
	Entries []*Entry
}

type Entry struct {
	Entry [32]byte
	Txn   [32]byte
}

func collect(db database.Beginner, partition config.NetworkUrl) (map[[32]byte]*Data, error) {
	fmt.Println("Scanning for entries")
	batch := db.Begin(false)
	defer batch.Discard()

	var ledger *protocol.SystemLedger
	err := batch.Account(partition.Ledger()).Main().GetAs(&ledger)
	if err != nil {
		return nil, errors.UnknownError.WithFormat("load system ledger: %w", err)
	}

	entries := map[[32]byte]*Data{}
	haveEntry := map[[2][32]byte]bool{}
	lastIndexIndex := map[[32]byte]uint64{}
	record := func(acct *url.URL, e *Entry) {
		if haveEntry[[2][32]byte{acct.AccountID32(), e.Txn}] {
			return
		}

		d, ok := entries[acct.AccountID32()]
		if !ok {
			d = new(Data)
			d.Account = acct
			entries[acct.AccountID32()] = d
		}

		d.Entries = append(d.Entries, e)
		haveEntry[[2][32]byte{acct.AccountID32(), e.Txn}] = true
	}

	defer fmt.Printf(" done\n")
	for i := uint64(protocol.GenesisBlock); i <= ledger.Index; i++ {
		const N, M = 50_000, 50
		if i%N == 0 {
			print(".")
		}
		if i%(N*M) == 0 {
			println()
		}
		var block *protocol.BlockLedger
		err = batch.Account(partition.BlockLedger(i)).Main().GetAs(&block)
		switch {
		case err == nil:
			// Ok
		case errors.Is(err, errors.NotFound):
			continue
		default:
			return nil, errors.UnknownError.WithFormat("load block %d: %w", i, err)
		}

		var votes = partition.JoinPath(protocol.Votes).AccountID32()
		var evidence = partition.JoinPath(protocol.Evidence).AccountID32()
		for _, e := range block.Entries {
			// Do not attempt to index Factom LDAs
			if i == protocol.GenesisBlock {
				_, err := protocol.ParseLiteAddress(e.Account)
				if err == nil {
					continue
				}
			}

			// Skip votes and evidence because something weird is going on and
			// may cause the index update to fail 🤷
			id := e.Account.AccountID32()
			if id == votes || id == evidence {
				continue
			}

			// Only care about the main chains
			if e.Chain != "main" && e.Chain != "scratch" {
				continue
			}

			// Is a data account?
			account, err := batch.Account(e.Account).Main().Get()
			switch {
			case err == nil:
				// Ok
			case errors.Is(err, errors.NotFound):
				continue
			default:
				return nil, errors.UnknownError.WithFormat("load %v: %w", e.Account, err)
			}
			switch account.Type() {
			case protocol.AccountTypeDataAccount,
				protocol.AccountTypeLiteDataAccount:
				// Ok
			default:
				continue
			}

			chain, err := batch.Account(e.Account).ChainByName(e.Chain)
			if err != nil {
				return nil, errors.UnknownError.WithFormat("get %v %s chain: %w", e.Account, e.Chain, err)
			}

			ic, err := chain.Index().Get()
			if err != nil {
				return nil, errors.UnknownError.WithFormat("get %v %s index chain: %w", e.Account, e.Chain, err)
			}
			index, entry, err := indexing.SearchIndexChain(ic, lastIndexIndex[e.Account.AccountID32()], indexing.MatchExact, indexing.SearchIndexChainBySource(e.Index))
			if err != nil {
				return nil, errors.UnknownError.WithFormat("find %v %s index chain entry for height %d: %w", e.Account, e.Chain, e.Index, err)
			}
			lastIndexIndex[e.Account.AccountID32()] = index

			var prev uint64
			if index > 0 {
				entry := new(protocol.IndexEntry)
				err = ic.EntryAs(int64(index-1), entry)
				if err != nil {
					return nil, errors.UnknownError.WithFormat("load %v %s index chain entry %d: %w", e.Account, e.Chain, index-1, err)
				}
				prev = entry.Source + 1
			}

			for i := prev; i <= entry.Source; i++ {
				txnHash, err := chain.Inner().Entry(int64(i))
				if err != nil {
					return nil, errors.UnknownError.WithFormat("get %v %s chain entry %d: %w", e.Account, e.Chain, i, err)
				}

				var msg messaging.MessageWithTransaction
				err = batch.Message2(txnHash).Main().GetAs(&msg)
				switch {
				case errors.Is(err, errors.NotFound):
					fmt.Printf("Cannot load state of %v %s chain entry %d (%x)\n", e.Account, e.Chain, i, txnHash)
					continue
				case err != nil:
					return nil, errors.UnknownError.WithFormat("load %v %s chain entry %d state: %w", e.Account, e.Chain, i, err)
				}

				var entryHash []byte
				switch body := msg.GetTransaction().Body.(type) {
				case *protocol.WriteData:
					entryHash = body.Entry.Hash()
				case *protocol.SyntheticWriteData:
					entryHash = body.Entry.Hash()
				case *protocol.SystemWriteData:
					entryHash = body.Entry.Hash()
				default:
					// Don't care
					continue
				}

				record(e.Account, &Entry{
					Entry: *(*[32]byte)(entryHash),
					Txn:   *(*[32]byte)(txnHash),
				})
			}
		}
	}
	return entries, nil
}

func fatalf(format string, args ...interface{}) {
	fmt.Fprintf(os.Stderr, "Error: "+format+"\n", args...)
	os.Exit(1)
}

func check(err error) {
	if err != nil {
		fatalf("%v", err)
	}
}
