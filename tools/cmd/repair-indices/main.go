package main

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
	"gitlab.com/accumulatenetwork/accumulate/config"
	"gitlab.com/accumulatenetwork/accumulate/internal/accumulated"
	"gitlab.com/accumulatenetwork/accumulate/internal/database"
	"gitlab.com/accumulatenetwork/accumulate/pkg/errors"
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
		return errors.Wrap(errors.UnknownError, err)
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
				return errors.Wrap(errors.UnknownError, err)
			}
		}

		err = r.Data().Entry().Overwrite(entryHashes)
		if err != nil {
			return errors.Wrap(errors.UnknownError, err)
		}
	}

	err = batch.Commit()
	return errors.Wrap(errors.UnknownError, err)
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
		return nil, errors.Format(errors.UnknownError, "load system ledger: %w", err)
	}

	entries := map[[32]byte]*Data{}
	record := func(acct *url.URL, e *Entry) {
		d, ok := entries[acct.AccountID32()]
		if !ok {
			d = new(Data)
			d.Account = acct
			entries[acct.AccountID32()] = d
		}

		d.Entries = append(d.Entries, e)
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
			return nil, errors.Format(errors.UnknownError, "load block %d: %w", i, err)
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

			// Only care about the main chain
			if e.Chain != "main" {
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
				return nil, errors.Format(errors.UnknownError, "load %v: %w", e.Account, err)
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
				return nil, errors.Format(errors.UnknownError, "get %v %s chain: %w", e.Account, e.Chain, err)
			}

			txnHash, err := chain.Inner().Get(int64(e.Index))
			if err != nil {
				return nil, errors.Format(errors.UnknownError, "get %v %s chain entry %d: %w", e.Account, e.Chain, e.Index, err)
			}

			state, err := batch.Transaction(txnHash).Main().Get()
			switch {
			case errors.Is(err, errors.NotFound):
				fmt.Printf("Cannot load state of %v %s chain entry %d (%x)\n", e.Account, e.Chain, e.Index, txnHash)
				continue
			case err != nil:
				return nil, errors.Format(errors.UnknownError, "load %v %s chain entry %d state: %w", e.Account, e.Chain, e.Index, err)
			case state.Transaction == nil:
				return nil, errors.Format(errors.UnknownError, "%v %s chain entry %d is not a transaction: %w", e.Account, e.Chain, e.Index, err)
			}

			var entryHash []byte
			switch body := state.Transaction.Body.(type) {
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
