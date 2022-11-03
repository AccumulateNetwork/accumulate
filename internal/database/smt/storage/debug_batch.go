// Copyright 2022 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package storage

import (
	"gitlab.com/accumulatenetwork/accumulate/internal/errors"
	"gitlab.com/accumulatenetwork/accumulate/internal/logging"
)

// debug is a bit field for enabling debug log messages
//nolint
const debug = 0 |
	// debugGet |
	// debugGetValue |
	// debugPut |
	// debugPutValue |
	0

const (
	// debugGet logs the key of DebugBatch.Get
	debugGet = 1 << iota
	// debugGetValue logs the value of DebugBatch.Get
	debugGetValue
	// debugPut logs the key of DebugBatch.Put
	debugPut
	// debugPutValue logs the value of DebugBatch.Put
	debugPutValue
)

type DebugBatch struct {
	Batch  KeyValueTxn
	Logger Logger

	Writable  bool
	didWrite  bool
	didCommit bool
}

var _ KeyValueTxn = (*DebugBatch)(nil)

func (b *DebugBatch) Begin(writable bool) KeyValueTxn {
	c := new(DebugBatch)
	c.Batch = b.Batch.Begin(writable)
	c.Logger = b.Logger
	c.Writable = b.Writable && writable
	return c
}

func (b *DebugBatch) Put(key Key, value []byte) error {
	switch debug & (debugPut | debugPutValue) {
	case debugPut | debugPutValue:
		b.Logger.Debug("Put", "key", key, "value", logging.AsHex(value))
	case debugPut:
		b.Logger.Debug("Put", "key", key)
	case debugPutValue:
		b.Logger.Debug("Put", "value", logging.AsHex(value))
	}

	b.didWrite = true
	return b.Batch.Put(key, value)
}

func (b *DebugBatch) PutAll(values map[Key][]byte) error {
	b.didWrite = true
	return b.Batch.PutAll(values)
}

func (b *DebugBatch) Get(key Key) (v []byte, err error) {
	switch debug & (debugGet | debugGetValue) {
	case debugGet | debugGetValue:
		defer func() {
			if err != nil {
				b.Logger.Debug("Get", "key", key, "value", err)
			} else {
				b.Logger.Debug("Get", "key", key, "value", logging.AsHex(v))
			}
		}()
	case debugGet:
		b.Logger.Debug("Get", "key", key)
	case debugGetValue:
		defer func() {
			if err != nil {
				b.Logger.Debug("Get", "error", err)
			} else {
				b.Logger.Debug("Get", "value", logging.AsHex(v))
			}
		}()
	}

	v, err = b.Batch.Get(key)
	if err != nil {
		return nil, errors.Wrap(errors.StatusUnknownError, err)
	}
	return v, nil
}

func (b *DebugBatch) PretendWrite() {
	b.didWrite = true
}

func (b *DebugBatch) Commit() error {
	if !b.Writable {
		b.Logger.Debug("Committing a read-only batch with no writes")
	} else if !b.didWrite {
		b.Logger.Debug("Committing a batch with no writes")
	}
	b.didCommit = true
	return b.Batch.Commit()
}

func (b *DebugBatch) Discard() {
	if b.didWrite && !b.didCommit {
		b.Logger.Debug("Discarding a writable batch with writes")
	}
	b.Batch.Discard()
}
