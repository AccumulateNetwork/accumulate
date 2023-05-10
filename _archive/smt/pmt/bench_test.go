// Copyright 2023 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package pmt

import (
	"testing"

	"github.com/stretchr/testify/require"
	"gitlab.com/accumulatenetwork/accumulate/internal/database/smt/common"
)

func BenchmarkInsert(b *testing.B) {
	bpt := NewBPTManager(nil)

	var rh common.RandHash
	for i := 0; i < b.N; i++ {
		bpt.InsertKV(rh.NextA(), rh.NextA())
	}
	require.NoError(b, bpt.Bpt.Update())
}
