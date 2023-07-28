// Copyright 2023 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package bpt

import (
	"encoding/hex"
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/tendermint/tendermint/libs/log"
	"gitlab.com/accumulatenetwork/accumulate/exp/lxrand"
	"gitlab.com/accumulatenetwork/accumulate/internal/database/record"
	"gitlab.com/accumulatenetwork/accumulate/internal/database/smt/common"
	"gitlab.com/accumulatenetwork/accumulate/internal/database/smt/storage"
	"gitlab.com/accumulatenetwork/accumulate/pkg/database"
	"gitlab.com/accumulatenetwork/accumulate/pkg/database/keyvalue"
	"gitlab.com/accumulatenetwork/accumulate/pkg/database/keyvalue/memory"
	"gitlab.com/accumulatenetwork/accumulate/pkg/database/values"
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
	// Manually construct a tree
	root := new(branch)
	root.bpt = New(nil, nil, nilStore{}, nil, "")
	root.Key, _ = nodeKeyAt(0, [32]byte{})
	for _, e := range testEntries {
		_, err := root.insert(&leaf{Key: e[0], Hash: e[1]})
		must(err)
	}
	h, _ := root.getHash()
	return h
}()

// TestInsertDirect inserts values, commits to the key-value store, recreates
// the model and inserts the last value, and verifies the root hash of the root
// batch's BPT.
func TestInsertDirect(t *testing.T) {
	store := memory.New(nil).Begin(nil, true)
	model := new(ChangeSet)
	model.store = keyvalue.RecordStore{Store: store}

	n := len(testEntries)
	for _, e := range testEntries[:n-1] {
		require.NoError(t, model.BPT().Insert(e[0], e[1]))
	}
	require.NoError(t, model.Commit())

	model = new(ChangeSet)
	model.store = keyvalue.RecordStore{Store: store}
	require.NoError(t, model.BPT().Insert(testEntries[n-1][0], testEntries[n-1][1]))
	root, err := model.BPT().GetRootHash()
	require.NoError(t, err)
	require.True(t, testRoot == root, "Expected root to match")
}

// TestInsertNested inserts values into a root batch, inserts values into a
// child batch, commits the child batch, and verifies the root hash of the root
// batch's BPT.
func TestInsertNested(t *testing.T) {
	store := memory.New(nil).Begin(nil, true)
	model := new(ChangeSet)
	model.store = keyvalue.RecordStore{Store: store}

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
	store := memory.New(nil).Begin(nil, true)
	model := new(ChangeSet)
	model.store = keyvalue.RecordStore{Store: store}

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
	store := memory.New(nil).Begin(nil, true)
	model := new(ChangeSet)
	model.store = keyvalue.RecordStore{Store: store}

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
	model.store = keyvalue.RecordStore{Store: store}
	var actual [][2][32]byte
	require.NoError(t, ForEach(model.BPT(), func(key storage.Key, hash [32]byte) error {
		actual = append(actual, [2][32]byte{key, hash})
		return nil
	}))
	require.True(t, len(expect) == len(actual), "Expect the same number of items")
	require.ElementsMatch(t, expect, actual)
}

// TestTreelessCommit verifies that a BPT that has not yet loaded any nodes is
// committed without constructing a tree.
func TestTreelessCommit(t *testing.T) {
	kvs := memory.New(nil)
	b1 := newBPT(nil, nil, keyvalue.RecordStore{Store: kvs.Begin(nil, true)}, nil, "BPT", "BPT")
	require.Empty(t, b1.pending)

	rs := testTreelessCommitRecordStore{t, b1}
	b2 := newBPT(nil, nil, rs, nil, "BPT", "BPT")
	require.NoError(t, b2.Insert([32]byte{1}, [32]byte{2}))
	require.NoError(t, b2.Commit())
	require.NotEmpty(t, b1.pending)
}

func TestDeleteAcrossBoundary(t *testing.T) {
	kvs := memory.New(nil).Begin(nil, true)
	store := keyvalue.RecordStore{Store: kvs}
	bpt := newBPT(nil, nil, store, nil, "BPT", "BPT")

	// Add the test data
	for _, e := range testEntries {
		require.NoError(t, bpt.Insert(e[0], e[1]))
	}

	// Add a key in the root block
	K1 := testEntries[0][0]
	K1[0] = 0x08
	require.NoError(t, bpt.Insert(K1, [32]byte{'a'}))
	require.NoError(t, bpt.Commit())

	Print(t, bpt, false)
	// ┬┬─    c000
	// │╰─    8000
	// ╰┬─    4000
	//  ╰┬─   ∅
	//   ╰┬─  ∅
	//    ╰┬─ 0800
	//     ╰─ 0000

	// Add a key that will be in another block
	K2 := testEntries[0][0]
	K2[1] = 0x80
	require.NoError(t, bpt.Insert(K2, [32]byte{'a'}))
	require.NoError(t, bpt.Commit())

	Print(t, bpt, false)
	// ┬┬─        c000
	// │╰─        8000
	// ╰┬─        4000
	//  ╰┬─       ∅
	//   ╰┬─      ∅
	//    ╰┬─     0800
	//     ╰┬─    ∅
	//      ╰┬─   ∅
	//       ╰┬─  ∅
	//        ╰╥─ 0080
	//         ╙─ 0000

	// Verify the second block was created
	_, err := kvs.Get(record.NewKey([32]byte{0x00, 0x80}))
	require.NoError(t, err)

	// Verify the height
	l, err := bpt.getRoot().getLeaf(K2)
	require.NoError(t, err)
	require.Equal(t, 8, int(l.parent.Height))

	// Unload the tree and delete the key
	bpt = newBPT(nil, nil, store, nil, "BPT", "BPT")
	require.NoError(t, bpt.Delete(K2))
	require.NoError(t, bpt.Commit())

	// Unload the tree and verify that the key is not found
	bpt = newBPT(nil, nil, store, nil, "BPT", "BPT")
	_, err = bpt.Get(K2)
	require.ErrorIs(t, err, errors.NotFound)

	Print(t, bpt, false)
	// ┬┬─   c000
	// │╰─   8000
	// ╰┬─   4000
	//  ╰┬─  ∅
	//   ╰┬─ ∅
	//    ╰┬─0800
	//     ╰─0000

	// Unload the tree and add another, different key that will be in the 0080
	// block
	bpt = newBPT(nil, nil, store, nil, "BPT", "BPT")
	K3 := testEntries[0][0]
	K3[1] = 0xC0
	require.NoError(t, bpt.Insert(K3, [32]byte{'a'}))
	require.NoError(t, bpt.Commit())

	// Verify that K2 is not found
	bpt = newBPT(nil, nil, store, nil, "BPT", "BPT")
	_, err = bpt.Get(K2)
	require.ErrorIs(t, err, errors.NotFound)

	Print(t, bpt, false)
	// ┬┬─       c000
	// │╰─       8000
	// ╰┬─       4000
	//  ╰┬─      ∅
	//   ╰┬─     ∅
	//    ╰┬─    0800
	//     ╰┬─   ∅
	//      ╰┬─  ∅
	//       ╰┬─ ∅
	//        ╰╥─00c0
	//         ╙─0000
}

func TestDeleteNested(t *testing.T) {
	store := memory.New(nil).Begin(nil, true)
	model := new(ChangeSet)
	model.store = keyvalue.RecordStore{Store: store}

	// Populate the BPT
	for _, e := range testEntries {
		require.NoError(t, model.BPT().Insert(e[0], e[1]))
	}

	before, err := model.BPT().GetRootHash()
	require.NoError(t, err)

	// Add entries
	A, B := [32]byte{0x20}, [32]byte{0x30}
	err = model.BPT().Insert(A, [32]byte{'a'})
	require.NoError(t, err)
	err = model.BPT().Insert(B, [32]byte{'b'})
	require.NoError(t, err)

	// Force the tree to load
	_, err = model.BPT().GetRootHash()
	require.NoError(t, err)

	// Create a child batch
	sub := model.Begin()

	// Delete the added entries in the child
	err = sub.BPT().Delete(A)
	require.NoError(t, err)
	err = sub.BPT().Delete(B)
	require.NoError(t, err)

	// Force the tree to load and verify the hash
	after, err := sub.BPT().GetRootHash()
	require.NoError(t, err)
	require.Equal(t, before, after)

	// Commit the child
	err = sub.Commit()
	require.NoError(t, err)

	// This must return not found
	_, err = model.BPT().Get(A)
	require.ErrorIs(t, err, errors.NotFound)

	// Verify the root hash returns to the previous value
	after, err = model.BPT().GetRootHash()
	require.NoError(t, err)
	require.Equal(t, before, after)
}

func TestDelete(t *testing.T) {
	var rand lxrand.Sequence
	store := keyvalue.RecordStore{Store: memory.New(nil).Begin(nil, true)}
	bpt := newBPT(nil, nil, store, nil, "BPT", "BPT")

	// Populate the BPT
	hashes := make([][32]byte, 50000)
	for i := range hashes {
		h := rand.Hash()
		hashes[i] = h
		require.NoError(t, bpt.Insert(h, h))
	}
	require.NoError(t, bpt.Commit())

	before, err := bpt.GetRootHash()
	require.NoError(t, err)

	// Reload
	bpt = newBPT(nil, nil, store, nil, "BPT", "BPT")

	// Randomly delete 50% of the entries
	deleted := make([][32]byte, len(hashes)/2)
	for i := range deleted {
		h := hashes[rand.Uint()%len(deleted)]
		deleted[i] = h
		require.NoError(t, bpt.Delete(h))
	}
	require.NoError(t, bpt.Commit())

	// Verify the hashes do not match
	after, err := bpt.GetRootHash()
	require.NoError(t, err)
	require.NotEqual(t, hex.EncodeToString(before[:]), hex.EncodeToString(after[:]))

	// Reload
	bpt = newBPT(nil, nil, store, nil, "BPT", "BPT")

	// Add the deleted entries back
	for _, h := range deleted {
		require.NoError(t, bpt.Insert(h, h))
	}
	require.NoError(t, bpt.Commit())

	// Verify the hashes match
	after, err = bpt.GetRootHash()
	require.NoError(t, err)
	require.Equal(t, hex.EncodeToString(before[:]), hex.EncodeToString(after[:]))
}

func newBPT(parent record.Record, logger log.Logger, store record.Store, key *record.Key, name, label string) *BPT {
	return New(parent, logger, store, key, label)
}

func (c *ChangeSet) Begin() *ChangeSet {
	d := new(ChangeSet)
	d.logger = c.logger
	d.store = values.RecordStore{Record: c}
	return d
}

type testTreelessCommitRecordStore struct {
	t      testing.TB
	record record.Record
}

func (s testTreelessCommitRecordStore) Unwrap() record.Record { return s.record }

func (s testTreelessCommitRecordStore) GetValue(key *database.Key, value database.Value) error {
	s.t.Helper()
	s.t.Fatalf("Unexpected call to GetValue")
	panic("not reached")
}

func (s testTreelessCommitRecordStore) PutValue(key *database.Key, value database.Value) error {
	s.t.Helper()
	s.t.Fatalf("Unexpected call to PutValue")
	panic("not reached")
}

type nilStore struct{}

var _ database.Store = nilStore{}

func (nilStore) GetValue(key *record.Key, value database.Value) error { return errors.NotFound }
func (nilStore) PutValue(key *record.Key, value database.Value) error { panic("no!") }

func TestPrint(t *testing.T) {
	var rand lxrand.Sequence
	store := keyvalue.RecordStore{Store: memory.New(nil).Begin(nil, true)}
	bpt := newBPT(nil, nil, store, nil, "BPT", "BPT")

	for i := 0; i < 50; i++ {
		require.NoError(t, bpt.Insert(rand.Hash(), rand.Hash()))
	}

	Print(t, bpt, false)
}

func PrintWithHeights(t *testing.T, b *BPT) {
	var print func(b *branch, p1, p2 string)
	print = func(b *branch, p1, p2 string) {
		fmt.Printf("%s╮{%X, height = %d}\n", p1, b.Key[:(b.Height/8)+1], b.Height)

		require.NoError(t, b.load())

		switch n := b.Left.(type) {
		case *emptyNode:
			fmt.Printf("%s├─∅\n", p2)
		case *leaf:
			fmt.Printf("%s├─%x → %x\n", p2, n.Key, n.Hash)
		case *branch:
			print(n, p2+"├", p2+"│")
		default:
			t.Fatalf("Unknown node type %T", n)
			panic("not reached")
		}

		switch n := b.Right.(type) {
		case *emptyNode:
			fmt.Printf("%s╰─∅\n", p2)
		case *leaf:
			fmt.Printf("%s╰─%x → %x\n", p2, n.Key, n.Hash)
		case *branch:
			print(n, p2+"╰", p2+" ")
		default:
			t.Fatalf("Unknown node type %T", n)
			panic("not reached")
		}
	}

	require.NoError(t, b.Commit())
	print(b.getRoot(), "", "")
}

func Print(t *testing.T, b *BPT, values bool) {
	var print func(b *branch, p1, p2 string)
	print = func(b *branch, p1, p2 string) {
		require.NoError(t, b.load()) // ╫

		var lp, sp, rp string
		if b.Height != 0 && b.Height%8 == 0 {
			// lp, sp, rp = "╥", "║", "╙"
			lp, sp, rp = "┰", "┃", "┖"
		} else {
			lp, sp, rp = "┬", "│", "╰"
		}

		switch n := b.Left.(type) {
		case *emptyNode:
			fmt.Printf("%s%s─∅\n", p1, lp)
		case *leaf:
			if values {
				fmt.Printf("%s%s─%x → %x\n", p1, lp, n.Key, n.Hash)
			} else {
				fmt.Printf("%s%s─%x\n", p1, lp, n.Key)
			}
		case *branch:
			print(n, p1+lp, p2+sp)
		default:
			t.Fatalf("Unknown node type %T", n)
			panic("not reached")
		}

		switch n := b.Right.(type) {
		case *emptyNode:
			fmt.Printf("%s%s─∅\n", p2, rp)
		case *leaf:
			if values {
				fmt.Printf("%s%s─%x → %x\n", p2, rp, n.Key, n.Hash)
			} else {
				fmt.Printf("%s%s─%x\n", p2, rp, n.Key)
			}
		case *branch:
			print(n, p2+rp, p2+" ")
		default:
			t.Fatalf("Unknown node type %T", n)
			panic("not reached")
		}
	}

	require.NoError(t, b.Commit())
	print(b.getRoot(), "", "")
}
