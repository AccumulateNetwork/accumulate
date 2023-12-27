// Copyright 2023 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package kvtest

import (
	"crypto/rand"
	"fmt"
	"io"
	"testing"

	"github.com/stretchr/testify/require"
	"gitlab.com/accumulatenetwork/accumulate/pkg/database"
	"gitlab.com/accumulatenetwork/accumulate/pkg/database/keyvalue"
	"gitlab.com/accumulatenetwork/accumulate/pkg/errors"
	"gitlab.com/accumulatenetwork/accumulate/pkg/types/record"
)

type Opener = func() (keyvalue.Beginner, error)

type closableDb struct {
	keyvalue.Beginner
	t      testing.TB
	closed bool
}

func (c *closableDb) Close() {
	if c.closed {
		return
	}
	c.closed = true

	if d, ok := c.Beginner.(io.Closer); ok {
		require.NoError(c.t, d.Close())
	}
}

func openDb(t testing.TB, open Opener) *closableDb {
	db, err := open()
	require.NoError(t, err)
	c := &closableDb{db, t, false}
	t.Cleanup(c.Close)
	return c
}

func TestDatabase(t *testing.T, open Opener) {
	const N = 10000

	// Open and write changes
	db := openDb(t, open)

	batch := db.Begin(nil, true)
	defer batch.Discard()

	// Read when nothing exists
	_, err := batch.Get(record.NewKey("answer", 0))
	require.Error(t, err)
	require.ErrorAs(t, err, new(*database.NotFoundError))

	// Write
	values := map[record.KeyHash]string{}
	for i := 0; i < N; i++ {
		key := record.NewKey("answer", i)
		value := fmt.Sprintf("%x this much data ", i)
		values[key.Hash()] = value
		err := batch.Put(key, []byte(value))
		require.NoError(t, err, "Put")
	}

	// Commit
	require.NoError(t, batch.Commit())

	// Verify with a new batch
	batch = db.Begin(nil, false)
	defer batch.Discard()

	for i := 0; i < N; i++ {
		val, err := batch.Get(record.NewKey("answer", i))
		require.NoError(t, err, "Get")
		require.Equal(t, fmt.Sprintf("%x this much data ", i), string(val))
	}

	batch.Discard()

	// Verify with a fresh instance
	db.Close()
	db = openDb(t, open)

	batch = db.Begin(nil, false)
	defer batch.Discard()

	for i := 0; i < N; i++ {
		val, err := batch.Get(record.NewKey("answer", i))
		require.NoError(t, err, "Get")
		require.Equal(t, fmt.Sprintf("%x this much data ", i), string(val))
	}

	// Verify ForEach
	require.NoError(t, batch.ForEach(func(key *record.Key, value []byte) error {
		expect, ok := values[key.Hash()]
		require.Truef(t, ok, "%v should exist", key)
		require.Equalf(t, expect, string(value), "%v should match", key)
		delete(values, key.Hash())
		return nil
	}))
	require.Empty(t, values, "All values should be iterated over")
}

func TestIsolation(t *testing.T, open Opener) {
	// Open and write
	db := openDb(t, open)

	batch := db.Begin(nil, true)
	defer batch.Discard()

	key := record.NewKey("key")
	err := batch.Put(key, []byte("value"))
	require.NoError(t, err, "Put")
	require.NoError(t, batch.Commit())

	// Start two batches
	b1 := db.Begin(nil, true)
	defer b1.Discard()

	b2 := db.Begin(nil, false)
	defer b2.Discard()

	// Delete and commit in batch 1
	require.NoError(t, b1.Delete(key))
	require.NoError(t, b1.Commit())

	// Verify the change is not visible from batch 2
	v, err := b2.Get(key)
	require.NoError(t, err, "Get")
	require.Equal(t, []byte("value"), v)

	// Verify the change is now visible
	batch = db.Begin(nil, true)
	defer batch.Discard()
	_, err = batch.Get(key)
	require.ErrorIs(t, err, errors.NotFound)
}

func TestSubBatch(t *testing.T, open Opener) {
	db := openDb(t, open)

	batch := db.Begin(nil, true)
	defer batch.Discard()
	sub := batch.Begin(nil, true)
	defer sub.Discard()

	for i := 0; i < 10000; i++ {
		err := sub.Put(record.NewKey("answer", i), []byte(fmt.Sprintf("%x this much data ", i)))
		require.NoError(t, err, "Put")
	}

	// Commit and begin a new sub-batch
	require.NoError(t, sub.Commit())
	sub = batch.Begin(nil, true)
	defer sub.Discard()

	for i := 0; i < 10000; i++ {
		val, err := sub.Get(record.NewKey("answer", i))
		require.NoError(t, err, "Get")
		require.Equal(t, fmt.Sprintf("%x this much data ", i), string(val))
	}
}

func TestPrefix(t *testing.T, open Opener) {
	data := make([]byte, 10)
	_, err := io.ReadFull(rand.Reader, data)
	require.NoError(t, err)

	db := openDb(t, open)

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

func TestDelete(t *testing.T, open Opener) {
	db := openDb(t, open)

	// Write a value
	batch := db.Begin(nil, true)
	defer batch.Discard()
	require.NoError(t, batch.Put(record.NewKey("foo"), []byte("bar")))
	require.NoError(t, batch.Commit())

	// Verify it can be retrieved
	batch = db.Begin(nil, false)
	defer batch.Discard()
	v, err := batch.Get(record.NewKey("foo"))
	require.NoError(t, err)
	require.Equal(t, "bar", string(v))
	batch.Discard()

	// Delete the value
	batch = db.Begin(nil, true)
	defer batch.Discard()
	require.NoError(t, batch.Delete(record.NewKey("foo")))

	// Verify it returns not found from the same batch
	_, err = batch.Get(record.NewKey("foo"))
	require.ErrorIs(t, err, errors.NotFound)

	// Commit and reopen
	require.NoError(t, batch.Commit())
	db.Close()
	db = openDb(t, open)

	// Verify it returns not found from a new batch
	batch = db.Begin(nil, false)
	defer batch.Discard()
	_, err = batch.Get(record.NewKey("foo"))
	require.ErrorIs(t, err, errors.NotFound)
}
