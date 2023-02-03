// Copyright 2023 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package snapshot

import (
	"io"

	"github.com/tendermint/tendermint/libs/log"
	"gitlab.com/accumulatenetwork/accumulate/internal/database"
	"gitlab.com/accumulatenetwork/accumulate/internal/node/config"
	ioutil2 "gitlab.com/accumulatenetwork/accumulate/internal/util/io"
	"gitlab.com/accumulatenetwork/accumulate/pkg/errors"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

// FullCollect collects a snapshot including additional records required for a
// fully-functioning node.
func FullCollect(batch *database.Batch, file io.WriteSeeker, network config.NetworkUrl, logger log.Logger, preserve bool) error {
	var ledger *protocol.SystemLedger
	err := batch.Account(network.Ledger()).Main().GetAs(&ledger)
	if err != nil {
		return errors.UnknownError.WithFormat("load system ledger: %w", err)
	}

	header := new(Header)
	header.Height = ledger.Index
	header.Timestamp = ledger.Timestamp
	header.ExecutorVersion = ledger.ExecutorVersion

	w, err := Collect(batch, header, file, CollectOptions{
		Logger: logger,
		PreserveAccountHistory: func(account *database.Account) (bool, error) {
			if preserve {
				return true, nil
			}

			// Preserve history for DN/BVN ADIs
			_, ok := protocol.ParsePartitionUrl(account.Url())
			return ok, nil
		},
	})
	if err != nil {
		return errors.UnknownError.Wrap(err)
	}

	err = CollectAnchors(w, batch, network)
	return errors.UnknownError.Wrap(err)
}

// CollectAnchors collects anchors from the anchor ledger's anchor sequence
// chain.
func CollectAnchors(w *Writer, batch *database.Batch, network config.NetworkUrl) error {
	txnHashes := new(HashSet)
	record := batch.Account(network.AnchorPool())
	err := txnHashes.CollectFromChain(record, record.AnchorSequenceChain())
	if err != nil {
		return errors.UnknownError.Wrap(err)
	}

	err = w.CollectTransactions(batch, txnHashes.Hashes, CollectOptions{})
	return errors.UnknownError.Wrap(err)
}

// FullRestore restores the snapshot and rebuilds indices.
func FullRestore(db database.Beginner, file ioutil2.SectionReader, logger log.Logger, network *config.Describe) error {
	err := Restore(db, file, logger)
	if err != nil {
		return errors.UnknownError.Wrap(err)
	}

	batch := db.Begin(true)
	defer batch.Discard()

	// Rebuild the synthetic transaction index index
	record := batch.Account(network.Synthetic())
	synthIndexChain, err := record.MainChain().Index().Get()
	if err != nil {
		return errors.InternalError.WithFormat("load synthetic index chain: %w", err)
	}

	entries, err := synthIndexChain.Entries(0, synthIndexChain.Height())
	if err != nil {
		return errors.InternalError.WithFormat("load synthetic index chain entries: %w", err)
	}

	for i, data := range entries {
		entry := new(protocol.IndexEntry)
		err = entry.UnmarshalBinary(data)
		if err != nil {
			return errors.InternalError.WithFormat("unmarshal synthetic index chain entry %d: %w", i, err)
		}

		err = batch.SystemData(network.PartitionId).SyntheticIndexIndex(entry.BlockIndex).Put(uint64(i))
		if err != nil {
			return errors.UnknownError.WithFormat("store synthetic transaction index index %d for block: %w", i, err)
		}
	}

	err = batch.Commit()
	return errors.UnknownError.Wrap(err)
}
