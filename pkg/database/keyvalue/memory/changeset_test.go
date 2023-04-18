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
	"gitlab.com/accumulatenetwork/accumulate/pkg/errors"
	"gitlab.com/accumulatenetwork/accumulate/pkg/types/record"
)

func GetKey(key []byte) (dbKey [32]byte) {
	dbKey = sha256.Sum256(key)
	return dbKey
}

func TestDatabase(t *testing.T) {
	db := New(nil)
	batch := db.Begin(nil, true)
	defer batch.Discard()

	for i := 0; i < 10000; i++ {
		err := batch.Put(record.NewKey("answer", i), []byte(fmt.Sprintf("%x this much data ", i)))
		require.NoError(t, err, "Put")
	}

	// Commit and begin a new batch
	require.NoError(t, batch.Commit())
	batch = db.Begin(nil, true)
	defer batch.Discard()

	for i := 0; i < 10000; i++ {
		val, err := batch.Get(record.NewKey("answer", i))
		require.NoError(t, err, "Get")
		require.Equal(t, fmt.Sprintf("%x this much data ", i), string(val))
	}
}

func TestPrefix(t *testing.T) {
	data := make([]byte, 10)
	_, err := io.ReadFull(rand.Reader, data)
	require.NoError(t, err)

	db := New(nil)

	const prefix, key = "foo", "bar"
	batch := db.Begin(record.NewKey(prefix), true)
	defer batch.Discard()
	require.NoError(t, batch.Put(record.NewKey(key), data))
	require.NoError(t, batch.Commit())

	batch = db.Begin(record.NewKey(prefix), true)
	defer batch.Discard()
	v, err := batch.Get(record.NewKey(key))
	require.NoError(t, err)
	require.Equal(t, data, v)
}

func TestDelete(t *testing.T) {
	db := New(nil)

	// Write a value
	batch := db.Begin(nil, true)
	require.NoError(t, batch.Put(record.NewKey("foo"), []byte("bar")))
	require.NoError(t, batch.Commit())

	// Verify it can be retrieved
	batch = db.Begin(nil, false)
	v, err := batch.Get(record.NewKey("foo"))
	require.NoError(t, err)
	require.Equal(t, "bar", string(v))
	batch.Discard()

	// Delete the value
	batch = db.Begin(nil, true)
	require.NoError(t, batch.Delete(record.NewKey("foo")))
	require.NoError(t, batch.Commit())

	// Verify it returns not found
	batch = db.Begin(nil, false)
	_, err = batch.Get(record.NewKey("foo"))
	require.ErrorIs(t, err, errors.NotFound)
}
