package storage

import (
	"gitlab.com/accumulatenetwork/accumulate/internal/logging"
)

// debug is a bit field for enabling debug log messages
const debug = 0 // | debugGet | debugGetValue | debugPut | debugPutValue

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

func (b *DebugBatch) Begin() KeyValueTxn {
	c := new(DebugBatch)
	c.Batch = b.Batch.Begin()
	c.Logger = b.Logger
	c.Writable = b.Writable
	return c
}

func (b *DebugBatch) Put(key Key, value []byte) error {
	if debug&debugPut != 0 {
		b.Logger.Debug("Put", "key", key)
	}
	if debug&debugPutValue != 0 {
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

	return b.Batch.Get(key)
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
	if b.Writable && !b.didCommit {
		b.Logger.Debug("Discarding a writable batch")
	}
	b.Batch.Discard()
}
