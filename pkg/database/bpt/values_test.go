// Copyright 2025 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package bpt

import (
	"testing"

	"github.com/stretchr/testify/require"
	"gitlab.com/accumulatenetwork/accumulate/pkg/database/keyvalue"
	"gitlab.com/accumulatenetwork/accumulate/pkg/database/keyvalue/memory"
	"gitlab.com/accumulatenetwork/accumulate/pkg/types/record"
)

var testValues = []struct {
	Key   *record.Key
	Value []byte
}{
	{record.NewKey(record.KeyHash{0x00}), []byte{1}},
	{record.NewKey(record.KeyHash{0x80}), []byte{2}},
	{record.NewKey(record.KeyHash{0x40}), []byte{3}},
	{record.NewKey(record.KeyHash{0xC0}), []byte{4}},
}

func TestNonHashValues(t *testing.T) {
	store := memory.New(nil).Begin(nil, true)
	model := new(ChangeSet)
	model.store = keyvalue.RecordStore{Store: store}
	err := model.BPT().SetParams(Parameters{
		ArbitraryValues: true,
	})
	require.NoError(t, err)

	for _, e := range testValues {
		require.NoError(t, model.BPT().Insert(e.Key, e.Value))
	}
	require.NoError(t, model.Commit())

	lup := map[[32]byte][]byte{}
	for _, entry := range testValues {
		lup[entry.Key.Hash()] = entry.Value
	}

	for it := model.BPT().Iterate(100); it.Next(); {
		for _, leaf := range it.Value() {
			require.Equal(t, lup[leaf.Key.Hash()], leaf.Value)
		}
	}

	Print(t, model.BPT(), true)
}
