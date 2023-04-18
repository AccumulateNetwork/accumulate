// Copyright 2023 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package bpt

import (
	"testing"

	"github.com/stretchr/testify/require"
	"gitlab.com/accumulatenetwork/accumulate/internal/database/smt/common"
	"gitlab.com/accumulatenetwork/accumulate/pkg/database/keyvalue"
	"gitlab.com/accumulatenetwork/accumulate/pkg/database/keyvalue/memory"
)

func BenchmarkInsert(b *testing.B) {
	store := memory.New(nil).Begin(nil, true)
	model := new(ChangeSet)
	model.store = keyvalue.RecordStore{Store: store}
	bpt := model.BPT()

	var rh common.RandHash
	for i := 0; i < b.N; i++ {
		err := bpt.Insert(rh.NextA(), rh.NextA())
		if err != nil {
			b.Fatal(err)
		}
	}
	require.NoError(b, bpt.Commit())
}

func BenchmarkInsertSubbatch(b *testing.B) {
	store := memory.New(nil).Begin(nil, true)
	model := new(ChangeSet)
	model.store = keyvalue.RecordStore{Store: store}
	bpt := model.BPT()

	var rh common.RandHash
	for i := 0; i < b.N; i++ {
		sub := model.Begin()
		err := sub.BPT().Insert(rh.NextA(), rh.NextA())
		if err != nil {
			b.Fatal(err)
		}
		err = sub.Commit()
		if err != nil {
			b.Fatal(err)
		}
	}
	require.NoError(b, bpt.Commit())
}
