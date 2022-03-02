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
}

func (b *DebugBatch) Put(key Key, value []byte) error {
	if debug&debugPut != 0 {
		b.Logger.Debug("Put", "key", key)
	}
	if debug&debugPutValue != 0 {
		b.Logger.Debug("Put", "value", logging.AsHex(value))
	}

	return b.Batch.Put(key, value)
}

func (b *DebugBatch) PutAll(values map[Key][]byte) error {
	return b.Batch.PutAll(values)
}

func (b *DebugBatch) Get(key Key) (v []byte, err error) {
	if debug&debugGet != 0 {
		b.Logger.Debug("Get", "key", key)
	}
	if debug&debugGetValue != 0 {
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

func (b *DebugBatch) Commit() error {
	return b.Batch.Commit()
}

func (b *DebugBatch) Discard() {
	b.Batch.Discard()
}
