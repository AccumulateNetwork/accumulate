package state

import (
	"encoding/hex"

	"github.com/AccumulateNetwork/accumulate/smt/storage"
)

type Index string

const (
	DirectoryIndex Index = "Directory"
)

func (s *StateDB) Write(key storage.Key, value []byte) {
	s.mutex.Lock()
	s.mutex.Unlock()
	s.writes[key] = value
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

func (s *StateDB) WriteIndex(index Index, chain []byte, key interface{}, value []byte) {
	k := storage.ComputeKey(string(index), chain, key)
	s.logInfo("WriteIndex", "index", string(index), "chain", hex.EncodeToString(chain), "key", key, "value", hex.EncodeToString(value), "computed", hex.EncodeToString(k[:]))
	s.Write(k, value)
}

func (s *StateDB) GetIndex(index Index, chain []byte, key interface{}) ([]byte, error) {
	k := storage.ComputeKey(string(index), chain, key)
	s.logInfo("GetIndex", "index", string(index), "chain", hex.EncodeToString(chain), "key", key, "computed", hex.EncodeToString(k[:]))
	return s.Read(k)
}

func (tx *DBTransactional) Write(key storage.Key, value []byte) {
	tx.mutex.Lock()
	tx.mutex.Unlock()
	tx.writes[key] = value
}

func (tx *DBTransactional) Read(key storage.Key) ([]byte, error) {
	tx.mutex.Lock()
	w, ok := tx.writes[key]
	tx.mutex.Unlock()
	if ok {
		return w, nil
	}
	return tx.GetDB().Key(key).Get()
}

func (tx *DBTransactional) WriteIndex(index Index, chain []byte, key interface{}, value []byte) {
	k := storage.ComputeKey(string(index), chain, key)
	tx.logInfo("WriteIndex", "index", string(index), "chain", hex.EncodeToString(chain), "key", key, "value", hex.EncodeToString(value), "computed", hex.EncodeToString(k[:]))
	tx.Write(k, value)
}

func (tx *DBTransactional) GetIndex(index Index, chain []byte, key interface{}) ([]byte, error) {
	k := storage.ComputeKey(string(index), chain, key)
	tx.logInfo("GetIndex", "index", string(index), "chain", hex.EncodeToString(chain), "key", key, "computed", hex.EncodeToString(k[:]))
	return tx.Read(k)
}
