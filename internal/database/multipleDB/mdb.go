package multipleDB

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/dgraph-io/badger"
)

type MDB struct {
	Tx        *badger.Txn
	Cnt       int
	Size      int
	Directory string
	DBBlocks  []*DBBlock
	DB        *badger.DB
}

// OpenBase
// Open the base database (Badger, LevelDB, whatever)
func (m *MDB) OpenBase(directory string) (err error) {

	if _, err := os.Stat(directory); err != nil {
		if err := os.MkdirAll(directory, os.ModePerm); err != nil {
			return err
		}
	}
	m.Directory = directory

	badgerFile := filepath.Join(m.Directory, "badger")
	m.DB, err = badger.Open(badger.DefaultOptions(badgerFile))
	if err != nil {
		return err
	}
	m.Tx = m.DB.NewTransaction(true)
	return err
}

func (m *MDB) Close() error {
	if err := m.Tx.Commit(); err != nil {
		return err
	}

	m.Tx.Discard()
	if err := m.DB.Close(); err != nil {
		return err
	}

	for _, dbb := range m.DBBlocks {
		if err := dbb.Close(); err != nil {
			return err
		}
	}
	return nil
}

// OpenDBBlock
// Opens a number of DBBlocks for holding hashed or write only key/values.
// DBBlocks must be opened in order (1, 2, 3, ...)
func (m *MDB) OpenDBBlock(category int) (err error) {
	category--                       // Make the category zero based
	if len(m.DBBlocks) != category { // force the order
		return fmt.Errorf("categories must be specified in order: expected %d got %d", len(m.DBBlocks), category)
	}
	if len(m.Directory) == 0 {
		return fmt.Errorf("open the base key value store (badger) first.")
	}

	dbbDirectory := filepath.Join(m.Directory, "DBB")

	dbb, err := NewDBBlock(dbbDirectory, 1)
	if err != nil {
		return err
	}
	m.DBBlocks = append(m.DBBlocks, dbb)
	err = dbb.Open(true)
	return err
}

func (m *MDB) BaseWrite(key [32]byte, value []byte) error {
	m.Cnt++
	m.Size += len(value)
	if m.Cnt > 10000 || m.Size > 1024*200 {
		if err := m.Tx.Commit(); err != nil {
			return err
		}
		m.Tx.Discard()
		m.Tx = m.DB.NewTransaction(true)
		m.Cnt = 0
		m.Size = 0
	}

	if err := m.Tx.Set(key[:], value); err != nil {
		return err
	}

	return nil
}

// Write
// Write a key value pair to one of the Databases.
func (m *MDB) Write(category int, key [32]byte, value []byte) error {
	if category == 0 {
		err := m.BaseWrite(key, value)
		return err
	}

	category-- // DBBlocks start at 1 and go up.
	err := m.DBBlocks[0].Write(key, value)
	return err
}

// Read
// Read a key value pair from one of the Databases
func (m *MDB) Read(key [32]byte) (value []byte) {
	m.DB.View(func(txn *badger.Txn) error {

		item, err := txn.Get(key[:])
		if err != nil {
			panic("what to do about errors here")
		}

		err = item.Value(func(val []byte) error {
			value = append([]byte{}, val...) // pass the data out
			return nil
		})
		if err != nil {
			panic("what to do about errors here too")
		}
		return nil
	})
	return value
}
