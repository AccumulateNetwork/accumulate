// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package types_test

import (
	"testing"

	"github.com/stretchr/testify/require"
	"gitlab.com/accumulatenetwork/accumulate/pkg/consensus/types"
)

// TestUnmarshalBatchCountIsBoundedByTheData: a batch's transaction count
// is checked against the bytes that follow it before anything is allocated
// for it. Anyone can publish a batch (consensus.md, "Who may send consensus
// messages"), and four bytes claiming a million transactions cost every
// receiver a 24 MB allocation before the decode failed.
func TestUnmarshalBatchCountIsBoundedByTheData(t *testing.T) {
	claim := []byte{0x00, 0x0f, 0x42, 0x40} // 1,000,000 transactions, no data
	_, err := types.UnmarshalBatch(claim)
	require.Error(t, err)

	r := testing.Benchmark(func(b *testing.B) {
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			_, _ = types.UnmarshalBatch(claim)
		}
	})
	require.Less(t, r.AllocedBytesPerOp(), int64(512), "a four-byte batch message allocated %d bytes", r.AllocedBytesPerOp())
}
