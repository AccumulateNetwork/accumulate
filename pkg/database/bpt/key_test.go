// Copyright 2024 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package bpt

import (
	"encoding/hex"
	"testing"

	"github.com/stretchr/testify/require"
	"gitlab.com/accumulatenetwork/accumulate/pkg/database/keyvalue"
	"gitlab.com/accumulatenetwork/accumulate/pkg/database/keyvalue/memory"
	"gitlab.com/accumulatenetwork/accumulate/pkg/types/record"
)

// TestOldBptStillWorks verifies that the BPT will still work with data recorded
// using a previous version.
func TestOldBptStillWorks(t *testing.T) {
	unhex := func(s string) []byte {
		b, err := hex.DecodeString(s)
		require.NoError(t, err)
		return b
	}

	// Load the recorded entries into a database
	store := memory.New(nil)
	require.NoError(t, store.Import([]memory.Entry{
		{
			Key: record.NewKey("BPT", "Root"),
			Value: unhex(
				"010008000770e98b7497b5e25de11190dad4af85cd3d8fedd4a93061171e13e42c3855f37c"),
		},
		{
			Key: record.NewKey("BPT", [32]byte{0x80}),
			Value: unhex("" +
				"02c000000000000000000000000000000000000000000000000000000000000000017cd1f9868114" +
				"069f943607a9cd41d1c8b3317f44b2522133ae6f9ee17c28a52b03c0000000000000000000000000" +
				"00000000000000000000000000000000000000040000000000000000000000000000000000000000" +
				"00000000000000000000000380000000000000000000000000000000000000000000000000000000" +
				"00000000020000000000000000000000000000000000000000000000000000000000000002400000" +
				"0000000000000000000000000000000000000000000000000000000000015b0f47b11478610c5abc" +
				"bc912b34ac5f7f5740b6d8b27169b1eb2aea9f1c909c034000000000000000000000000000000000" +
				"00000000000000000000000000000003000000000000000000000000000000000000000000000000" +
				"00000000000000030000000000000000000000000000000000000000000000000000000000000000" +
				"0100000000000000000000000000000000000000000000000000000000000000"),
		},
	}))

	// Check the root
	model := new(ChangeSet)
	model.store = keyvalue.RecordStore{Store: store.Begin(nil, true)}
	root, err := model.BPT().GetRootHash()
	require.NoError(t, err)
	require.Equal(t, testRoot, root)

	// Check the entries
	for _, entry := range testEntries {
		value, err := model.BPT().Get(entry.Key)
		require.NoError(t, err)
		require.Equal(t, entry.Hash[:], value)
	}

	testEntryMap := map[[32]byte][32]byte{}
	for _, entry := range testEntries {
		testEntryMap[entry.Key.Hash()] = entry.Hash
	}

	for it := model.BPT().Iterate(100); it.Next(); {
		for _, leaf := range it.Value() {
			h := testEntryMap[leaf.Key.Hash()]
			require.Equal(t, h[:], leaf.Value)
		}
	}
}

// TestSwitchKeyType verifies that nothing explodes when BPT entry keys are
// changed from compressed to expanded.
func TestSwitchKeyType(t *testing.T) {
	kvs := memory.New(nil).Begin(nil, true)
	store := keyvalue.RecordStore{Store: kvs}
	bpt := newBPT(nil, nil, store, nil, "BPT")

	entries := []*record.Key{
		record.NewKey(1),
		record.NewKey(2),
		record.NewKey(3),
		record.NewKey(4),
	}

	// Insert entries using compressed keys
	for _, e := range entries {
		h := e.Hash()
		require.NoError(t, bpt.Insert(record.KeyFromHash(e.Hash()), h[:]))
	}
	require.NoError(t, bpt.Commit())
	before := allBptKeys(bpt)

	// Reload the tree and update using expanded keys
	bpt = newBPT(nil, nil, store, nil, "BPT")
	for _, e := range entries[:len(entries)/2] {
		h := e.Append("foo").Hash()
		require.NoError(t, bpt.Insert(e, h[:]))
	}
	require.NoError(t, bpt.Commit())

	// Verify the leaf now uses an expanded key
	l, err := bpt.getRoot().getLeaf(entries[0].Hash())
	require.NoError(t, err)
	require.True(t, isExpandedKey(l.Key))

	// Reload the tree and verify
	bpt = newBPT(nil, nil, store, nil, "BPT")
	l, err = bpt.getRoot().getLeaf(entries[0].Hash())
	require.NoError(t, err)
	require.True(t, isExpandedKey(l.Key))

	// Keys did not change
	after := allBptKeys(bpt)
	require.Equal(t, before, after)
}

func allBptKeys(bpt *BPT) [][32]byte {
	var entries [][32]byte
	for it := bpt.Iterate(100); it.Next(); {
		for _, leaf := range it.Value() {
			entries = append(entries, leaf.Key.Hash())
		}
	}
	return entries
}
