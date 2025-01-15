// Copyright 2025 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package badger

import (
	"os"

	v1 "github.com/dgraph-io/badger"
	v2 "github.com/dgraph-io/badger/v2"
	v3 "github.com/dgraph-io/badger/v3"
	v4 "github.com/dgraph-io/badger/v4"
	"gitlab.com/accumulatenetwork/accumulate/pkg/errors"
)

type DatabaseV1 = DB[*v1.DB, *v1.Txn, *v1.Item, *v1.WriteBatch]
type DatabaseV2 = DB[*v2.DB, *v2.Txn, *v2.Item, *v2.WriteBatch]
type DatabaseV3 = DB[*v3.DB, *v3.Txn, *v3.Item, *v3.WriteBatch]
type DatabaseV4 = DB[*v4.DB, *v4.Txn, *v4.Item, *v4.WriteBatch]

func OpenV1(filepath string, o ...Option) (*Database, error) {
	// Make sure all directories exist
	err := os.MkdirAll(filepath, 0700)
	if err != nil {
		return nil, errors.UnknownError.WithFormat("open badger: create %q: %w", filepath, err)
	}

	opts := v1.DefaultOptions(filepath)
	opts = opts.WithLogger(slogger{})

	// Truncate corrupted data
	if TruncateBadger {
		opts = opts.WithTruncate(true)
	}

	// Open Badger
	badger, err := v1.Open(opts)
	if err != nil {
		return nil, err
	}

	return open[*v1.DB, *v1.Txn, *v1.Item, *v1.WriteBatch](badger, args[*v1.Txn, *v1.Item]{
		newIterator: func(t *v1.Txn) iterator[*v1.Item] {
			opts := v1.DefaultIteratorOptions
			opts.PrefetchValues = true
			return t.NewIterator(opts)
		},
		errKeyNotFound: v1.ErrKeyNotFound,
		errNoRewrite:   v1.ErrNoRewrite,
	}, o)
}

func OpenV2(filepath string, o ...Option) (*DatabaseV2, error) {
	// Make sure all directories exist
	err := os.MkdirAll(filepath, 0700)
	if err != nil {
		return nil, errors.UnknownError.WithFormat("open badger: create %q: %w", filepath, err)
	}

	opts := v2.DefaultOptions(filepath)
	opts = opts.WithLogger(slogger{})

	// Truncate corrupted data
	if TruncateBadger {
		opts = opts.WithTruncate(true)
	}

	// Open Badger
	badger, err := v2.Open(opts)
	if err != nil {
		return nil, err
	}

	return open[*v2.DB, *v2.Txn, *v2.Item, *v2.WriteBatch](badger, args[*v2.Txn, *v2.Item]{
		newIterator: func(t *v2.Txn) iterator[*v2.Item] {
			opts := v2.DefaultIteratorOptions
			opts.PrefetchValues = true
			return t.NewIterator(opts)
		},
		errKeyNotFound: v2.ErrKeyNotFound,
		errNoRewrite:   v2.ErrNoRewrite,
	}, o)
}

func OpenV3(filepath string, o ...Option) (*DatabaseV3, error) {
	// Make sure all directories exist
	err := os.MkdirAll(filepath, 0700)
	if err != nil {
		return nil, errors.UnknownError.WithFormat("open badger: create %q: %w", filepath, err)
	}

	opts := v3.DefaultOptions(filepath)
	opts = opts.WithLogger(slogger{})

	// Open Badger
	badger, err := v3.Open(opts)
	if err != nil {
		return nil, err
	}

	return open[*v3.DB, *v3.Txn, *v3.Item, *v3.WriteBatch](badger, args[*v3.Txn, *v3.Item]{
		newIterator: func(t *v3.Txn) iterator[*v3.Item] {
			opts := v3.DefaultIteratorOptions
			opts.PrefetchValues = true
			return t.NewIterator(opts)
		},
		errKeyNotFound: v3.ErrKeyNotFound,
		errNoRewrite:   v3.ErrNoRewrite,
	}, o)
}

func OpenV4(filepath string, o ...Option) (*DatabaseV4, error) {
	// Make sure all directories exist
	err := os.MkdirAll(filepath, 0700)
	if err != nil {
		return nil, errors.UnknownError.WithFormat("open badger: create %q: %w", filepath, err)
	}

	opts := v4.DefaultOptions(filepath)
	opts = opts.WithLogger(slogger{})

	// Open Badger
	badger, err := v4.Open(opts)
	if err != nil {
		return nil, err
	}

	return open[*v4.DB, *v4.Txn, *v4.Item, *v4.WriteBatch](badger, args[*v4.Txn, *v4.Item]{
		newIterator: func(t *v4.Txn) iterator[*v4.Item] {
			opts := v4.DefaultIteratorOptions
			opts.PrefetchValues = true
			return t.NewIterator(opts)
		},
		errKeyNotFound: v4.ErrKeyNotFound,
		errNoRewrite:   v4.ErrNoRewrite,
	}, o)
}
