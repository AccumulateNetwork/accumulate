// Copyright 2022 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package snapshot

import (
	"io"

	"github.com/tendermint/tendermint/libs/log"
	"gitlab.com/accumulatenetwork/accumulate/config"
	"gitlab.com/accumulatenetwork/accumulate/internal/database"
	ioutil2 "gitlab.com/accumulatenetwork/accumulate/internal/ioutil"
	"gitlab.com/accumulatenetwork/accumulate/internal/logging"
	"gitlab.com/accumulatenetwork/accumulate/pkg/errors"
	"gitlab.com/accumulatenetwork/accumulate/pkg/url"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

// FullCollect collects a snapshot including additional records required for a
// fully-functioning node.
func FullCollect(batch *database.Batch, file io.WriteSeeker, network *config.Describe, logger log.Logger) error {
	var ledger *protocol.SystemLedger
	err := batch.Account(network.Ledger()).Main().GetAs(&ledger)
	if err != nil {
		return errors.Format(errors.UnknownError, "load system ledger: %w", err)
	}

	header := new(Header)
	header.Height = ledger.Index
	header.Timestamp = ledger.Timestamp

	w, err := Collect(batch, header, file, logger, func(account *database.Account) (bool, error) {
		// Preserve history for DN/BVN ADIs
		_, ok := protocol.ParsePartitionUrl(account.Url())
		return ok, nil
	})
	if err != nil {
		return errors.Wrap(errors.UnknownError, err)
	}

	err = CollectAnchors(w, batch, network)
	return errors.Wrap(errors.UnknownError, err)
}

// CollectAnchors collects anchors from the anchor ledger's anchor sequence
// chain.
func CollectAnchors(w *Writer, batch *database.Batch, network *config.Describe) error {
	txnHashes := new(HashSet)
	record := batch.Account(network.AnchorPool())
	err := txnHashes.CollectFromChain(record, record.AnchorSequenceChain())
	if err != nil {
		return errors.Wrap(errors.UnknownError, err)
	}

	err = w.CollectTransactions(batch, txnHashes.Hashes, nil)
	return errors.Wrap(errors.UnknownError, err)
}

// FullRestore restores the snapshot and rebuilds indices.
func FullRestore(db database.Beginner, file ioutil2.SectionReader, logger log.Logger, network *config.Describe) error {
	err := Restore(db, file, logger)
	if err != nil {
		return errors.Wrap(errors.UnknownError, err)
	}

	batch := db.Begin(true)
	defer batch.Discard()

	// Rebuild the synthetic transaction index index
	record := batch.Account(network.Synthetic())
	synthIndexChain, err := record.MainChain().Index().Get()
	if err != nil {
		return errors.Format(errors.InternalError, "load synthetic index chain: %w", err)
	}

	entries, err := synthIndexChain.Entries(0, synthIndexChain.Height())
	if err != nil {
		return errors.Format(errors.InternalError, "load synthetic index chain entries: %w", err)
	}

	for i, data := range entries {
		entry := new(protocol.IndexEntry)
		err = entry.UnmarshalBinary(data)
		if err != nil {
			return errors.Format(errors.InternalError, "unmarshal synthetic index chain entry %d: %w", i, err)
		}

		err = batch.SystemData(network.PartitionId).SyntheticIndexIndex(entry.BlockIndex).Put(uint64(i))
		if err != nil {
			return errors.Format(errors.UnknownError, "store synthetic transaction index index %d for block: %w", i, err)
		}
	}

	err = batch.Commit()
	if err != nil {
		return errors.Wrap(errors.UnknownError, err)
	}

	// Rebuild data indices
	err = rebuildDataIndices(db, network.PartitionUrl(), logger)
	if err != nil {
		// Data indices are non-essential so don't fail
		logger.Error("Unable to rebuild data indices", "error", err)
	}

	return nil
}

func rebuildDataIndices(db database.Beginner, partition config.NetworkUrl, logger log.Logger) error {
	accounts, err := collectDataEntries(db, partition, logger)
	if err != nil {
		return errors.Wrap(errors.UnknownError, err)
	}

	batch := db.Begin(true)
	defer batch.Discard()

	for _, a := range accounts {
		logger.Info("Rebuidling data index", "account", a.Account, "entries", len(a.Entries))
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

type dataEntries struct {
	Account *url.URL
	Entries []*dataEntry
}

type dataEntry struct {
	Entry [32]byte
	Txn   [32]byte
}

func collectDataEntries(db database.Beginner, partition config.NetworkUrl, logger log.Logger) (map[[32]byte]*dataEntries, error) {
	logger.Info("Scanning for data entries")
	batch := db.Begin(false)
	defer batch.Discard()

	var ledger *protocol.SystemLedger
	err := batch.Account(partition.Ledger()).Main().GetAs(&ledger)
	if err != nil {
		return nil, errors.Format(errors.UnknownError, "load system ledger: %w", err)
	}

	entries := map[[32]byte]*dataEntries{}
	record := func(acct *url.URL, e *dataEntry) {
		d, ok := entries[acct.AccountID32()]
		if !ok {
			d = new(dataEntries)
			d.Account = acct
			entries[acct.AccountID32()] = d
		}

		d.Entries = append(d.Entries, e)
	}

	for i := uint64(protocol.GenesisBlock); i <= ledger.Index; i++ {
		if i > 0 && i%100_000 == 0 {
			logger.Info("Scanned blocks", "count", i, "total", ledger.Index)
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
				logger.Info("Cannot load state of chain entry", "account", e.Account, "chain", e.Chain, "index", e.Index, "hash", logging.AsHex(txnHash))
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

			record(e.Account, &dataEntry{
				Entry: *(*[32]byte)(entryHash),
				Txn:   *(*[32]byte)(txnHash),
			})
		}
	}
	return entries, nil
}
