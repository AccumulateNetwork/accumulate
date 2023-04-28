// Copyright 2023 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package database

import (
	"io"

	"gitlab.com/accumulatenetwork/accumulate/pkg/database/snapshot"
	"gitlab.com/accumulatenetwork/accumulate/pkg/errors"
	"gitlab.com/accumulatenetwork/accumulate/pkg/types/record"
	"gitlab.com/accumulatenetwork/accumulate/pkg/url"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

type CollectOptions struct {
	PreserveAll bool
}

func (db *Database) Collect(file io.WriteSeeker, partition *url.URL, opts *CollectOptions) error {
	// Walk records, recording their key, value, and offset. Build a list of
	// (key hash, offset) pairs, sort it, and store that as the index.

	if opts == nil {
		opts = new(CollectOptions)
	}

	rootBatch := db.Begin(false)
	defer rootBatch.Discard()

	var ledger *protocol.SystemLedger
	err := rootBatch.Account(partition.JoinPath(protocol.Ledger)).Main().GetAs(&ledger)
	if err != nil {
		return errors.UnknownError.WithFormat("load system ledger: %w", err)
	}

	rootHash, err := rootBatch.BPT().GetRootHash()
	if err != nil {
		return errors.UnknownError.WithFormat("get root hash: %w", err)
	}

	w, err := snapshot.Create(file)
	if err != nil {
		return errors.UnknownError.WithFormat("open snapshot: %w", err)
	}

	err = w.WriteHeader(&snapshot.Header{
		RootHash:     rootHash,
		SystemLedger: ledger,
	})
	if err != nil {
		return errors.UnknownError.WithFormat("write header: %w", err)
	}

	records, err := w.Open()
	if err != nil {
		return errors.UnknownError.WithFormat("open record section: %w", err)
	}

	it := rootBatch.BPT().Iterate(1000)
	for {
		entries, ok := it.Next()
		if !ok {
			break
		}

		batch := db.Begin(false)
		defer batch.Discard()
		for _, v := range entries {
			u, err := batch.getAccountUrl(record.NewKey(record.KeyHash(v.Key)))
			if err != nil {
				return errors.UnknownError.WithFormat("resolve key hash: %w", err)
			}
			err = records.Collect(batch.Account(u))
			if err != nil {
				return errors.UnknownError.WithFormat("collect %v: %w", u, err)
			}
		}
	}
	if it.Err() != nil {
		return errors.UnknownError.Wrap(it.Err())
	}

	err = records.Close()
	if err != nil {
		return errors.UnknownError.Wrap(err)
	}

	err = w.WriteIndex()
	if err != nil {
		return errors.UnknownError.WithFormat("write index: %w", err)
	}

	return nil
}
