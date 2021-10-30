package state

import (
	"encoding/hex"

	"github.com/AccumulateNetwork/accumulated/smt/storage"
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

func (s *StateDB) WriteIndex(index Index, chain []byte, key interface{}, value []byte) {
	k := storage.ComputeKey(string(index), chain, key)
	s.logInfo("WriteIndex", "index", string(index), "chain", hex.EncodeToString(chain), "key", key, "value", hex.EncodeToString(value), "computed", hex.EncodeToString(k[:]))
	s.Write(k, value)
}

func (s *StateDB) GetIndex(index Index, chain []byte, key interface{}) ([]byte, error) {
	k := storage.ComputeKey(string(index), chain, key)
	s.logInfo("GetIndex", "index", string(index), "chain", hex.EncodeToString(chain), "key", key, "computed", hex.EncodeToString(k[:]))
	s.mutex.Lock()
	w, ok := s.writes[k]
	s.mutex.Unlock()
	if ok {
		return w, nil
	}
	return s.GetDB().Key(k).Get()
}
