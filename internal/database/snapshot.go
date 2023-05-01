// Copyright 2023 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package database

import (
	"io"

	"gitlab.com/accumulatenetwork/accumulate/pkg/database"
	"gitlab.com/accumulatenetwork/accumulate/pkg/database/snapshot"
	"gitlab.com/accumulatenetwork/accumulate/pkg/errors"
	"gitlab.com/accumulatenetwork/accumulate/pkg/url"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

type CollectOptions struct {
	PreserveAll bool
}

func (db *Database) Collect(file io.WriteSeeker, partition *url.URL, opts *CollectOptions) error {
	if opts == nil {
		opts = new(CollectOptions)
	}

	// Start the snapshot
	w, err := snapshot.Create(file)
	if err != nil {
		return errors.UnknownError.WithFormat("open snapshot: %w", err)
	}

	// Write the header
	err = db.writeSnapshotHeader(w, partition, opts)
	if err != nil {
		return errors.UnknownError.Wrap(err)
	}

	// Collect accounts
	err = db.collectAccounts(w, opts)
	if err != nil {
		return errors.UnknownError.Wrap(err)
	}

	err = w.WriteIndex()
	if err != nil {
		return errors.UnknownError.WithFormat("write index: %w", err)
	}

	return nil
}

func (db *Database) writeSnapshotHeader(w *snapshot.Writer, partition *url.URL, opts *CollectOptions) error {
	// Load the ledger
	batch := db.Begin(false)
	defer batch.Discard()

	var ledger *protocol.SystemLedger
	err := batch.Account(partition.JoinPath(protocol.Ledger)).Main().GetAs(&ledger)
	if err != nil {
		return errors.UnknownError.WithFormat("load system ledger: %w", err)
	}

	// Load the BPT root hash
	rootHash, err := batch.BPT().GetRootHash()
	if err != nil {
		return errors.UnknownError.WithFormat("get root hash: %w", err)
	}

	// Write the header
	err = w.WriteHeader(&snapshot.Header{
		RootHash:     rootHash,
		SystemLedger: ledger,
	})
	if err != nil {
		return errors.UnknownError.WithFormat("write header: %w", err)
	}

	return nil
}

func (db *Database) collectAccounts(w *snapshot.Writer, opts *CollectOptions) error {
	// Open a records section
	records, err := w.Open()
	if err != nil {
		return errors.UnknownError.WithFormat("open record section: %w", err)
	}

	// Iterate over the BPT and collect accounts
	batch := db.Begin(false)
	defer batch.Discard()
	it := batch.IterateAccounts()
	for {
		account, ok := it.Next()
		if !ok {
			break
		}

		err = records.Collect(account, database.WalkOptions{
			IgnoreIndices: true,
		})
		if err != nil {
			return errors.UnknownError.WithFormat("collect %v: %w", account.Url(), err)
		}
	}
	if it.Err() != nil {
		return errors.UnknownError.Wrap(it.Err())
	}

	err = records.Close()
	return errors.UnknownError.Wrap(err)
}
