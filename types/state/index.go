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
	tx.state.writes[key] = value
}

func (s *StateDB) Read(key storage.Key) ([]byte, error) {
	s.mutex.Lock()
	w, ok := s.writes[key]
	s.mutex.Unlock()
	if ok {
		return w, nil
	}
	return s.GetDB().Key(key).Get()
}

func (tx *DBTransaction) Read(key storage.Key) ([]byte, error) {
	tx.state.mutex.Lock()
	w, ok := tx.state.writes[key]
	tx.state.mutex.Unlock()
	if ok {
		return w, nil
	}
	return tx.state.GetDB().Key(key).Get()
}

func (tx *DBTransaction) WriteIndex(index Index, chain []byte, key interface{}, value []byte) {
	k := storage.ComputeKey(string(index), chain, key)
	tx.state.logInfo("WriteIndex", "index", string(index), "chain", hex.EncodeToString(chain), "key", key, "value", hex.EncodeToString(value), "computed", hex.EncodeToString(k[:]))
	tx.Write(k, value)
}

func (s *StateDB) GetIndex(index Index, chain []byte, key interface{}) ([]byte, error) {
	k := storage.ComputeKey(string(index), chain, key)
	s.logInfo("GetIndex", "index", string(index), "chain", hex.EncodeToString(chain), "key", key, "computed", hex.EncodeToString(k[:]))
	return s.Read(k)
}

func (tx *DBTransaction) GetIndex(index Index, chain []byte, key interface{}) ([]byte, error) {
	k := storage.ComputeKey(string(index), chain, key)
	tx.state.logInfo("GetIndex", "index", string(index), "chain", hex.EncodeToString(chain), "key", key, "computed", hex.EncodeToString(k[:]))
	return tx.Read(k)
}
