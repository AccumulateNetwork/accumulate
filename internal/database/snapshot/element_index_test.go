// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package snapshot_test

import (
	"testing"

	"github.com/stretchr/testify/require"
	"gitlab.com/accumulatenetwork/accumulate/internal/database/record"
	"gitlab.com/accumulatenetwork/accumulate/internal/database/smt/common"
	"gitlab.com/accumulatenetwork/accumulate/internal/database/snapshot"
	"gitlab.com/accumulatenetwork/accumulate/pkg/database/keyvalue"
	"gitlab.com/accumulatenetwork/accumulate/pkg/database/keyvalue/memory"
	"gitlab.com/accumulatenetwork/accumulate/pkg/database/merkle"
)

// The element index a restore writes names the position the chain holds the
// hash at: AddEntry(unique) refutes an index by the element it names (#4327),
// so an index offset from the entry would append a hash the chain holds
// (#4321).
func TestRestoredElementIndexNamesThePositionOfTheEntry(t *testing.T) {
	newChain := func(key string) *merkle.Chain {
		store := keyvalue.RecordStore{Store: memory.New(nil).Begin(nil, true)}
		return merkle.NewChain(nil, store, record.NewKey(key), 8, merkle.ChainTypeTransaction, "main")
	}
	inner := newChain("src")

	const n = 700 // past two mark points, with an open set
	var rh common.RandHash
	var hashes [][]byte
	for i := 0; i < n; i++ {
		h := rh.NextList()
		hashes = append(hashes, h)
		require.NoError(t, inner.AddEntry(h, false))
	}

	head, err := inner.Head().Get()
	require.NoError(t, err)
	var marks []*merkle.State
	for c := inner.MarkFreq(); c <= head.Count; c += inner.MarkFreq() {
		st, err := inner.States(uint64(c - 1)).Get()
		require.NoError(t, err)
		marks = append(marks, st)
	}
	snap := &snapshot.Chain{Name: "main", Type: merkle.ChainTypeTransaction, MarkPower: uint64(inner.MarkPower()), Head: head, MarkPoints: marks}

	dst := newChain("dst")
	require.NoError(t, snap.RestoreElementIndexFromMarkPoints(dst, 0, len(marks)))
	require.NoError(t, snap.RestoreElementIndexFromHead(dst))

	for i, h := range hashes {
		got, err := dst.IndexOf(h)
		if err != nil {
			continue // a mark point may hold no hashes
		}
		require.Equal(t, int64(i), got, "the restored index of entry %d names another position", i)
	}
}
