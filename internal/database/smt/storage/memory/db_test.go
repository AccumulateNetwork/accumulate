// Copyright 2023 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package memory

import (
	"crypto/rand"
	"crypto/sha256"
	"fmt"
	"io"
	"testing"

	"github.com/stretchr/testify/require"
	"gitlab.com/accumulatenetwork/accumulate/internal/database/smt/storage"
)

func GetKey(key []byte) (dbKey [32]byte) {
	dbKey = sha256.Sum256(key)
	return dbKey
}

func TestDatabase(t *testing.T) {
	db := New(nil)
	batch := db.Begin(true)
	defer batch.Discard()

	for i := 0; i < 10000; i++ {
		err := batch.Put(GetKey([]byte(fmt.Sprintf("answer %d", i))), []byte(fmt.Sprintf("%x this much data ", i)))
		if err != nil {
			t.Fatal(err)
		}
	}
	for i := 0; i < 10000; i++ {

		val, e := batch.Get(GetKey([]byte(fmt.Sprintf("answer %d", i))))
		if e != nil {
			t.Fatalf("no value found for %d", i)
		}

		if string(val) != fmt.Sprintf("%x this much data ", i) {
			t.Error("Did not read data properly")
		}
	}
}

func TestPrefix(t *testing.T) {
	data := make([]byte, 10)
	_, err := io.ReadFull(rand.Reader, data)
	require.NoError(t, err)

	db := New(nil)

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
