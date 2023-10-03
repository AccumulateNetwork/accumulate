// Copyright 2023 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package bpt

import (
	"bytes"
	"sort"
	"testing"

	"github.com/stretchr/testify/require"
	"gitlab.com/accumulatenetwork/accumulate/internal/database/smt/common"
	"gitlab.com/accumulatenetwork/accumulate/pkg/database/keyvalue"
	"gitlab.com/accumulatenetwork/accumulate/pkg/database/keyvalue/memory"
	"gitlab.com/accumulatenetwork/accumulate/pkg/types/record"
)

// TestBPT_receipt
// Build a reasonable size BPT, then prove we can create a receipt for every
// element in said BPT.
func TestBPT_receipt(t *testing.T) {
	kvs := memory.New(nil).Begin(nil, true)
	store := keyvalue.RecordStore{Store: kvs}
	bpt := newBPT(nil, nil, store, nil, "BPT")

	numberEntries := 50000 //               A pretty reasonable sized BPT

	var keys, values common.RandHash     //     use the default sequence for keys
	values.SetSeed([]byte{1, 2, 3})      //     use a different sequence for values
	for i := 0; i < numberEntries; i++ { // For the number of Entries specified for the BPT
		chainID := keys.NextAList()                           //      Get a key, keep a list
		value := values.NextA()                               //      Get some value (don't really care what it is)
		err := bpt.Insert(record.KeyFromHash(chainID), value) //      Insert the Key with the value into the BPT
		require.NoError(t, err)
	}
	require.NoError(t, bpt.Commit())

	keyList := append([][]byte{}, keys.List...)                                                   // Get all the keys
	sort.Slice(keyList, func(i, j int) bool { return bytes.Compare(keyList[i], keyList[j]) < 0 }) //

	// Recreate the BPT to throw away the pending list, otherwise GetReceipt
	// spends a lot of time iterating over it and doing nothing
	bpt = newBPT(nil, nil, store, nil, "BPT")

	// Make sure every key we added to the BPT has a valid receipt
	for i := range keys.List { // go through the list of keys
		r, err := bpt.GetReceipt(record.KeyFromHash(keys.GetAElement(i)))
		require.NoError(t, err)
		v := r.Validate(nil)
		require.Truef(t, v, "should validate BPT element %d", i+1)
	}
}
