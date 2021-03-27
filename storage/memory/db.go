package memory

import (
	"sync"

	"github.com/AccumulateNetwork/SMT/storage"
)

type DB struct {
	entries map[[storage.KeyLength]byte][]byte
	mutex   sync.Mutex
}

func (m *DB) PutBatch(txList []storage.TX) error {
	for _, v := range txList {
		var key [32]byte
		copy(key[:], v.Key)
		_ = m.Put(key, v.Value)
	}
	return nil
}

func (m *DB) Close() error {
	return nil
}
func (m *DB) InitDB(string) error {
	m.entries = make(map[[storage.KeyLength]byte][]byte)
	return nil
}
func (m *DB) Get(key [storage.KeyLength]byte) (value []byte) {
	m.mutex.Lock()
	defer m.mutex.Unlock()
	return m.entries[key]
}
func (m *DB) Put(key [storage.KeyLength]byte, value []byte) error {
	m.mutex.Lock()
	defer m.mutex.Unlock()
	m.entries[key] = value
	return nil
}
