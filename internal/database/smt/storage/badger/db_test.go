// Copyright 2023 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package badger

import (
	"crypto/rand"
	"fmt"
	"io"
	"os"
	"testing"

	"github.com/dgraph-io/badger"
	"github.com/stretchr/testify/require"
	"gitlab.com/accumulatenetwork/accumulate/internal/database/smt/storage"
)

func TestPrefix(t *testing.T) {
	data := make([]byte, 10)
	_, err := io.ReadFull(rand.Reader, data)
	require.NoError(t, err)

	db, err := New(t.TempDir(), nil)
	require.NoError(t, err)

	const prefix, key = "foo", "bar"
	batch := db.BeginWithPrefix(true, prefix)
	defer batch.Discard()
	require.NoError(t, batch.Put(storage.MakeKey(key), data))
	require.NoError(t, batch.Commit())

	batch = db.BeginWithPrefix(true, prefix)
	defer batch.Discard()
	v, err := batch.Get(storage.MakeKey(key))
	require.NoError(t, err)
	require.Equal(t, data, v)
}

func TestDatabase(t *testing.T) {
	dname, e := os.MkdirTemp("", "sampledir")
	if e != nil {
		t.Fatal(e)
	}
	defer os.RemoveAll(dname)

	db, err := badger.Open(badger.DefaultOptions(dname))
	if err != nil {
		t.Fatal(err.Error())
	}

	defer db.Close()

	for i := 0; i < 10000; i++ {
		err = db.Update(func(txn *badger.Txn) error {
			err := txn.Set([]byte(fmt.Sprintf("answer %d", i)), []byte(fmt.Sprintf("%x this much data ", i)))
			return err
		})
		if err != nil {
			t.Fatal(err)
		}
	}
	//fmt.Println("Reads")
	for i := 0; i < 10000; i++ {
		var val []byte
		err = db.View(func(txn *badger.Txn) error {
			item, err := txn.Get([]byte(fmt.Sprintf("answer %d", i)))
			require.NoError(t, err)
			err = item.Value(func(v []byte) error {
				val = append(val, v...)
				return nil
			})
			require.NoError(t, err)
			return nil
		})
		require.NoError(t, err)

		if string(val) != fmt.Sprintf("%x this much data ", i) {
			t.Error("Did not read data properly")
		}
	}
}

func TestDatabase2(t *testing.T) {
	dname, e := os.MkdirTemp("", "sampledir")
	if e != nil {
		t.Fatal(e)
	}
	defer os.RemoveAll(dname)

	db, err := badger.Open(badger.DefaultOptions(dname))
	if err != nil {
		t.Fatal(err.Error())
	}

	defer db.Close()

	txn := db.NewTransaction(true)
	for i := 0; i < 10000; i++ {
		if err := txn.Set([]byte(fmt.Sprintf("answer %d", i)), []byte(fmt.Sprintf("%x this much data ", i))); err != nil {
			t.Fatal(err)
		}
	}
	if err := txn.Commit(); err != nil {
		t.Fatal(err)
	}
	//fmt.Println("Reads")
	for i := 0; i < 10000; i++ {
		var val []byte
		err = db.View(func(txn *badger.Txn) error {
			item, err := txn.Get([]byte(fmt.Sprintf("answer %d", i)))
			require.NoError(t, err)
			err = item.Value(func(v []byte) error {
				val = append(val, v...)
				return nil
			})
			require.NoError(t, err)
			return nil
		})
		require.NoError(t, err)

		if string(val) != fmt.Sprintf("%x this much data ", i) {
			t.Error("Did not read data properly")
		}
	}
}
