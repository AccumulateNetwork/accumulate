package testdb

import (
	"bytes"
	"encoding/binary"
	"os"
	"path/filepath"
	"testing"

	"github.com/dgraph-io/badger"
)

type keyValue struct {
	key  [32]byte
	data []byte
}

type SDB struct {
	Directory  string
	StaticFile string
	DB         *badger.DB
	Data       *os.File
	Keys       *os.File
	UseFile    bool
	EOF        uint64
}

func (s *SDB) Close(t *testing.T) error {
	return s.DB.Close()
}

func (s *SDB) Open(directory string) error {
	s.Directory = directory
	db, err := badger.Open(badger.DefaultOptions(s.Directory))
	if err != nil {
		return err
	}
	s.DB = db

	tmpfile := filepath.Join(s.Directory, "Records.dat")
	s.Data, err = os.Create(tmpfile)
	if err != nil {
		db.Close()
		s.Data = nil
		s.DB = nil
		return err
	}

	tmpfile = filepath.Join(s.Directory, "Keys.dat")
	s.Keys, err = os.Create(tmpfile)
	if err != nil {
		s.Data.Close()
		db.Close()
		s.Keys = nil
		s.Data = nil
		s.DB = nil
		return err
	}
	return nil
}

func (s *SDB) Write(dataSet []keyValue) error {
	if !s.UseFile {
		txn := s.DB.NewTransaction(true)
		for _, kv := range dataSet {
			if err := txn.Set(kv.key[:], kv.data); err != nil {
				return err
			}
		}
		if err := txn.Commit(); err != nil {
			return err
		}
		txn.Discard()

	} else {
		var lengthBuff [16]byte
		var data bytes.Buffer
		var keys bytes.Buffer
		txn := s.DB.NewTransaction(true)
		for _, kv := range dataSet {
			binary.BigEndian.PutUint64(lengthBuff[:], s.EOF)
			length, err := data.Write(kv.data)
			if err != nil {
				return err
			}
			if _,err := keys.Write(kv.key[:]); err != nil {
				return err
			}
			txn.Set(kv.key[:], lengthBuff[:])
			s.EOF += uint64(len(kv.data))
		}
		s.Data.Write(data.Bytes())
		s.Keys.Write(keys.Bytes())
		txn.Commit()
		txn.Discard()
	}
}
