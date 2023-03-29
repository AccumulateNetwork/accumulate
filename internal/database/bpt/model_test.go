// Copyright 2023 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package bpt

import (
	"fmt"
	"reflect"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/tendermint/tendermint/libs/log"
	"gitlab.com/accumulatenetwork/accumulate/internal/database/record"
	"gitlab.com/accumulatenetwork/accumulate/internal/database/smt/common"
	"gitlab.com/accumulatenetwork/accumulate/internal/database/smt/pmt"
	"gitlab.com/accumulatenetwork/accumulate/internal/database/smt/storage"
	"gitlab.com/accumulatenetwork/accumulate/internal/database/smt/storage/memory"
	"gitlab.com/accumulatenetwork/accumulate/pkg/errors"
)

var testEntries = [][2][32]byte{
	{{0x00}, {1}},
	{{0x80}, {2}},
	{{0x40}, {3}},
	{{0xC0}, {4}},
}

func must(err error) {
	if err != nil {
		panic(err)
	}
}

var testRoot = func() [32]byte {
	bpt := pmt.NewBPTManager(nil)
	for _, e := range testEntries {
		bpt.InsertKV(e[0], e[1])
	}
	must(bpt.Bpt.Update())
	return bpt.GetRootHash()
}()

// TestInsertDirect inserts values, commits to the key-value store, recreates
// the model and inserts the last value, and verifies the root hash of the root
// batch's BPT.
func TestInsertDirect(t *testing.T) {
	store := memory.New(nil).Begin(true)
	model := new(ChangeSet)
	model.store = record.KvStore{Store: store}

	n := len(testEntries)
	for _, e := range testEntries[:n-1] {
		require.NoError(t, model.BPT().Insert(e[0], e[1]))
	}
	require.NoError(t, model.Commit())

	model = new(ChangeSet)
	model.store = record.KvStore{Store: store}
	require.NoError(t, model.BPT().Insert(testEntries[n-1][0], testEntries[n-1][1]))
	root, err := model.BPT().GetRootHash()
	require.NoError(t, err)
	require.True(t, testRoot == root, "Expected root to match")
}

// TestInsertNested inserts values into a root batch, inserts values into a
// child batch, commits the child batch, and verifies the root hash of the root
// batch's BPT.
func TestInsertNested(t *testing.T) {
	store := memory.New(nil).Begin(true)
	model := new(ChangeSet)
	model.store = record.KvStore{Store: store}

	n := len(testEntries)
	for _, e := range testEntries[:n/2] {
		require.NoError(t, model.BPT().Insert(e[0], e[1]))
	}

	sub := model.Begin()
	hash, err := sub.BPT().Get(testEntries[0][0])
	require.NoError(t, err)
	require.Equal(t, testEntries[0][1], hash)

	for _, e := range testEntries[n/2:] {
		require.NoError(t, sub.BPT().Insert(e[0], e[1]))
	}
	require.NoError(t, sub.Commit())
	root, err := model.BPT().GetRootHash()
	require.NoError(t, err)
	require.True(t, testRoot == root, "Expected root to match")
}

// TestInsertConcurrent inserts values into multiple different parent and child
// batches, commits all the child batches, and verifies the root hash of the
// root batch's BPT.
func TestInsertConcurrent(t *testing.T) {
	e := testEntries
	store := memory.New(nil).Begin(true)
	model := new(ChangeSet)
	model.store = record.KvStore{Store: store}

	sub1 := model.Begin()
	sub2 := model.Begin()

	// Insert into root after creating a child batch
	require.NoError(t, model.BPT().Insert(e[0][0], e[0][1]))

	// Insert into child batches
	require.NoError(t, sub1.BPT().Insert(e[1][0], e[1][1]))
	require.NoError(t, sub2.BPT().Insert(e[2][0], e[2][1]))

	// Insert the remainder into the root batch
	for _, e := range e[3:] {
		require.NoError(t, model.BPT().Insert(e[0], e[1]))
	}

	// Commit the child batches
	require.NoError(t, sub1.Commit())
	require.NoError(t, sub2.Commit())

	// Verify
	root, err := model.BPT().GetRootHash()
	require.NoError(t, err)
	require.True(t, testRoot == root, "Expected root to match")
}

func TestRange(t *testing.T) {
	store := memory.New(nil).Begin(true)
	model := new(ChangeSet)
	model.store = record.KvStore{Store: store}

	sub := model.Begin()
	var rh common.RandHash
	var expect [][2][32]byte
	for i := 0; i < 100; i++ {
		key, hash := rh.NextA(), rh.NextA()
		require.NoError(t, sub.BPT().Insert(key, hash))
		expect = append(expect, [2][32]byte{key, hash})
	}
	require.NoError(t, sub.Commit())
	require.NoError(t, model.Commit())

	// Make sure there are enough entries to create multiple blocks
	s, err := model.BPT().getState().Get()
	require.NoError(t, err)
	fmt.Printf("Max height is %d\n", s.MaxHeight)
	require.Greater(t, s.MaxHeight, s.Power)

	model = new(ChangeSet)
	model.store = record.KvStore{Store: store}
	var actual [][2][32]byte
	require.NoError(t, model.BPT().ForEach(func(key storage.Key, hash [32]byte) error {
		actual = append(actual, [2][32]byte{key, hash})
		return nil
	}))
	require.True(t, len(expect) == len(actual), "Expect the same number of items")
	require.ElementsMatch(t, expect, actual)
}

func newBPT(parent record.Record, logger log.Logger, store record.Store, key record.Key, name, label string) *BPT {
	b := new(BPT)
	b.logger.Set(logger)
	b.store = store
	b.key = key
	b.label = label
	return b
}

func (c *ChangeSet) Begin() *ChangeSet {
	d := new(ChangeSet)
	d.logger = c.logger
	d.store = c
	return d
}

// GetValue implements record.Store.
func (c *ChangeSet) GetValue(key record.Key, value record.ValueWriter) error {
	v, err := resolveValue[record.ValueReader](c, key)
	if err != nil {
		return errors.UnknownError.Wrap(err)
	}

	err = value.LoadValue(v, false)
	return errors.UnknownError.Wrap(err)
}

// PutValue implements record.Store.
func (c *ChangeSet) PutValue(key record.Key, value record.ValueReader) error {
	v, err := resolveValue[record.ValueWriter](c, key)
	if err != nil {
		return errors.UnknownError.Wrap(err)
	}

	err = v.LoadValue(value, true)
	return errors.UnknownError.Wrap(err)
}

func zero[T any]() T {
	var z T
	return z
}

// resolveValue resolves the value for the given key.
func resolveValue[T any](c *ChangeSet, key record.Key) (T, error) {
	var r record.Record = c
	var err error
	for len(key) > 0 {
		r, key, err = r.Resolve(key)
		if err != nil {
			return zero[T](), errors.UnknownError.Wrap(err)
		}
	}

	if s, _, err := r.Resolve(nil); err == nil {
		r = s
	}

	v, ok := r.(T)
	if !ok {
		return zero[T](), errors.InternalError.WithFormat("bad key: %T is not a %v", r, reflect.TypeOf(new(T)).Elem())
	}

	return v, nil
}