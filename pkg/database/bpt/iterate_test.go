// Copyright 2024 The Accumulate Authors
// 
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package bpt

import (
	"bytes"
	"fmt"
	"sort"
	"testing"

	"github.com/stretchr/testify/require"
	"gitlab.com/accumulatenetwork/accumulate/internal/database/smt/common"
	"gitlab.com/accumulatenetwork/accumulate/pkg/database/keyvalue"
	"gitlab.com/accumulatenetwork/accumulate/pkg/database/keyvalue/memory"
	"gitlab.com/accumulatenetwork/accumulate/pkg/types/record"
)

var _ = fmt.Print

func TestGetRange(t *testing.T) {
	for i := 0; i < 15000; i += 1017 {
		GetRangeFor(t, i, 13)
	}
}

func GetRangeFor(t *testing.T, numberEntries, rangeNum int) {
	fmt.Println(numberEntries)
	kvs := memory.New(nil).Begin(nil, true)    //     Build a BPT
	store := keyvalue.RecordStore{Store: kvs}  //
	bpt := newBPT(nil, nil, store, nil, "BPT") //
	var keys, values common.RandHash           //     use the default sequence for keys
	values.SetSeed([]byte{1, 2, 3})            //     use a different sequence for values
	for i := 0; i < numberEntries; i++ {       // For the number of Entries specified for the BPT
		chainID := keys.NextAList()                           //      Get a key, keep a list
		value := values.NextA()                               //      Get some value (don't really care what it is)
		err := bpt.Insert(record.KeyFromHash(chainID), value) //      Insert the Key with the value into the BPT
		require.NoError(t, err)
	}

	cnt := 0

	// The BPT will sort the keys, so we take the list of keys we used, and sort them
	sort.Slice(keys.List, func(i, j int) bool { return bytes.Compare(keys.List[i], keys.List[j]) > 0 })

	// We ask for a range of rangeNum entries at a time.
	it := bpt.Iterate(13)
	for i := 0; i < numberEntries; i += rangeNum {
		require.True(t, it.Next(), "Must be more BPT entries")
		bptValues := it.Value()
		if len(bptValues) == 0 {
			break
		}
		for j, v := range bptValues {
			k := keys.List[i+j]
			h := v.Key.Hash()
			require.Truef(t, bytes.Equal(h[:], k), "i,j= %d:%d %02x should be %02x", i, j, h[:2], k[:2])
		}
		cnt += len(bptValues)
	}

	require.Equal(t, len(keys.List), cnt)
}
