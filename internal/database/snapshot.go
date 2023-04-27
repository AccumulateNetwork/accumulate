// Copyright 2023 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package database

// import (
// 	"io"

// 	"gitlab.com/accumulatenetwork/accumulate/pkg/errors"
// 	"gitlab.com/accumulatenetwork/accumulate/pkg/types/record"
// 	"gitlab.com/accumulatenetwork/accumulate/pkg/url"
// 	"gitlab.com/accumulatenetwork/accumulate/protocol"
// )

// type CollectOptions struct {
// 	PreserveAll bool
// }

// func (db *Database) Collect(file io.WriteSeeker, partition *url.URL, opts *CollectOptions) error {
// 	// Walk records, recording their key, value, and offset. Build a list of
// 	// (key hash, offset) pairs, sort it, and store that as the index.

// 	if opts == nil {
// 		opts = new(CollectOptions)
// 	}

// 	batch := db.Begin(false)
// 	defer batch.Discard()

// 	var ledger *protocol.SystemLedger
// 	err := batch.Account(partition.JoinPath(protocol.Ledger)).Main().GetAs(&ledger)
// 	if err != nil {
// 		return errors.UnknownError.WithFormat("load system ledger: %w", err)
// 	}

// 	err = batch.BPT().ForEach(func(key record.KeyHash, hash [32]byte) error {
// 		// Get the account URL
// 		u, err := batch.getAccountUrl(record.NewKey(key))
// 		if err != nil {
// 			return errors.UnknownError.Wrap(err)
// 		}
// 	})
// }
