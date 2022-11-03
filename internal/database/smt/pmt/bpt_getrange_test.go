// Copyright 2022 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package pmt_test

import (
	"bytes"
	"fmt"
	"sort"
	"testing"

	"github.com/stretchr/testify/require"
	"gitlab.com/accumulatenetwork/accumulate/internal/database/smt/common"
	. "gitlab.com/accumulatenetwork/accumulate/internal/database/smt/pmt"
)

var _ = fmt.Print

func TestGetRange(t *testing.T) {
	for i := 0; i < 50000; i += 1017 {
		GetRangeFor(t, i, 13)
	}
}

func GetRangeFor(t *testing.T, numberEntries, rangeNum int) {
	bpt := NewBPTManager(nil).Bpt        //     Build a BPT
	var keys, values common.RandHash     //     use the default sequence for keys
	values.SetSeed([]byte{1, 2, 3})      //     use a different sequence for values
	for i := 0; i < numberEntries; i++ { // For the number of Entries specified for the BPT
		chainID := keys.NextAList() //      Get a key, keep a list
		value := values.NextA()     //      Get some value (don't really care what it is)
		bpt.Insert(chainID, value)  //      Insert the Key with the value into the BPT
	}

	searchKey := [32]byte{ // Highest value for a key
		0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF,
		0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF,
	}
	var bptValues []*Value
	cnt := 0

	// The BPT will sort the keys, so we take the list of keys we used, and sort them
	sort.Slice(keys.List, func(i, j int) bool { return bytes.Compare(keys.List[i], keys.List[j]) > 0 })

	// We ask for a range of rangeNum entries at a time.
	for i := 0; i < numberEntries; i += rangeNum {
		bptValues, searchKey = bpt.GetRange(searchKey, rangeNum)
		if len(bptValues) == 0 {
			break
		}
		for j, v := range bptValues {
			k := keys.List[i+j]
			require.Truef(t, bytes.Equal(v.Key[:], k), "i,j= %d:%d %02x should be %02x", i, j, v.Key[:2], k[:2])
		}
		cnt += len(bptValues)
	}

	require.Equal(t, len(keys.List), cnt)
}
