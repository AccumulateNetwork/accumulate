package state

import (
	"encoding/hex"

	"github.com/AccumulateNetwork/accumulate/smt/storage"
)

type Index string

const (
	DirectoryIndex Index = "Directory"
)

func (tx *DBTransaction) Write(key storage.Key, value []byte) {
	tx.state.mutex.Lock()
	tx.state.mutex.Unlock()
	tx.writes[key] = value
}

func (tx *DBTransaction) Read(key storage.Key) ([]byte, error) {
	tx.state.mutex.Lock()
	w, ok := tx.writes[key]
	tx.state.mutex.Unlock()
	if ok {
		return w, nil
	}
	return tx.state.Read(key)
}

func (db *StateDB) Read(key storage.Key) ([]byte, error) {
	return db.GetDB().Key(key).Get()
}

func (tx *DBTransaction) WriteIndex(index Index, chain []byte, key interface{}, value []byte) {
	k := storage.ComputeKey(string(index), chain, key)
	tx.state.logDebug("WriteIndex", "index", string(index), "chain", hex.EncodeToString(chain), "key", key, "value", hex.EncodeToString(value), "computed", hex.EncodeToString(k[:]))
	tx.Write(k, value)
}

func (tx *DBTransaction) GetIndex(index Index, chain []byte, key interface{}) ([]byte, error) {
	k := storage.ComputeKey(string(index), chain, key)
	tx.state.logDebug("GetIndex", "index", string(index), "chain", hex.EncodeToString(chain), "key", key, "computed", hex.EncodeToString(k[:]))
	return tx.Read(k)
}

func (db *StateDB) GetIndex(index Index, chain []byte, key interface{}) ([]byte, error) {
	k := storage.ComputeKey(string(index), chain, key)
	db.logDebug("GetIndex", "index", string(index), "chain", hex.EncodeToString(chain), "key", key, "computed", hex.EncodeToString(k[:]))
	return db.Read(k)
}
