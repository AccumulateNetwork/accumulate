// Copyright 2024 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

// MultipleDB
// To manage a blockchain's Database more efficiently, we use multiple 
//   databases.  
//   - A DB: traditional Key Value Store for tracking the keys that matter
//     which depends on the utility required of the app
//   - A DBBlock data base as a flat file.  Holds a list of values and
//     a sorted list of keys and offsets into the DBBlock
//
//   In Accumulate, we need the DB to find the offsets over a large set of
//   DBBlocks efficiently.  The DB can be pruned heavily by validators 
//   which do not need the historical data
//
//   Data servers need all the keys in their DB
//  
// Provides special handling for types of key value pairs
//   - Keys that are updated are held in the underlying DB (badger)
//   - DBBlocks hold the key/value pairs where the key is the hash of the data
//   - key/value pair written to DBBlock file
//   - key and file/offset of data kept in DB instead of the data
//   - One DBBlock per major block
//   - Active DBBlock is built from a block file and a key file
//   - Closing a DBBlock puts moves at end of DBBlock
//   - Tossing a scratch DBBlock
//   - keys from the scratch DBBlock are purged from the DB
//   - scratch DBBlock is deleted
//   - Keys that are hashes of the data
//   - If part of the permanent blockchain, kept in permanent DBBlocks
//   - If scratch chains, kept in scratch DBBlocks
//
// Snapshot
//
//	All key/value pairs of the DB sans all DBBlock key/value pairs
//	DBBlocks are moved to data servers
//	Data queries go against the nodes, but if the key/value pair isn't found,
//	   the query is made against a database holding all the DBBlocks
package multipleDB

import (
	"bytes"
	"encoding/binary"
	"os"
	"path/filepath"
	"testing"

	"github.com/dgraph-io/badger"
)

//go:generate go run  gitlab.com/accumulatenetwork/accumulate/tools/cmd/gen-types --package multipleDB types.yml 


type keyValue struct {
	key  [32]byte
	data []byte
}

// 
type MultipleDB struct {
	Directory  string		// Directory holding the 
	StaticFile string
	DBBlock    uint64
	DB         *badger.DB
	Data       *os.File
	Keys       *os.File
	UseFile    bool
	EOF        uint64
}

func (s *MultipleDB) NewBlock() {
	

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
			_, err := data.Write(kv.data)
			if err != nil {
				return err
			}
			if _, err := keys.Write(kv.key[:]); err != nil {
				return err
			}
			txn.Set(kv.key[:], lengthBuff[:])
			s.EOF += uint64(len(kv.data))cd 
		}
		s.Data.Write(data.Bytes())
		s.Keys.Write(keys.Bytes())
		txn.Commit()
		txn.Discard()
	}
}
