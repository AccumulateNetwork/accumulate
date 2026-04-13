// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package proof_test

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	. "gitlab.com/accumulatenetwork/accumulate/internal/core/execute/v2/proof"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

func TestBatcher(t *testing.T) {
	batcher := NewBatcher(3)
	require.NotNil(t, batcher)

	// Add requests to the batcher
	var root [32]byte
	for i := 0; i < 32; i++ {
		root[i] = byte(i)
	}

	reqs := make([]*ProofRequest, 4)
	for i := 0; i < 4; i++ {
		reqs[i] = &ProofRequest{
			Account:   protocol.AccountUrl("alice"),
			Anchor:    protocol.PartitionUrl("Directory"),
			Sequence:  uint64(i),
			Root:      root,
			Timestamp: time.Now(),
		}
	}

	// Add first 3 requests
	batch1, full1 := batcher.AddRequest(reqs[0])
	require.Nil(t, batch1)
	require.False(t, full1)

	batch2, full2 := batcher.AddRequest(reqs[1])
	require.Nil(t, batch2)
	require.False(t, full2)

	batch3, full3 := batcher.AddRequest(reqs[2])
	require.NotNil(t, batch3)
	require.True(t, full3)
	require.Len(t, batch3.Requests, 3)

	// Add 4th request to new batch
	batch4, full4 := batcher.AddRequest(reqs[3])
	require.Nil(t, batch4)
	require.False(t, full4)

	// Flush remaining batches
	remaining := batcher.Flush()
	require.Len(t, remaining, 1)
	require.Len(t, remaining[0].Requests, 1)
}

func TestGroupByDestination(t *testing.T) {
	var root [32]byte
	reqs := []*ProofRequest{
		{
			Account:   protocol.AccountUrl("alice"),
			Anchor:    protocol.PartitionUrl("Directory"),
			Sequence:  1,
			Root:      root,
			Timestamp: time.Now(),
		},
		{
			Account:   protocol.AccountUrl("bob"),
			Anchor:    protocol.PartitionUrl("Validator0"),
			Sequence:  1,
			Root:      root,
			Timestamp: time.Now(),
		},
		{
			Account:   protocol.AccountUrl("charlie"),
			Anchor:    protocol.PartitionUrl("Directory"),
			Sequence:  2,
			Root:      root,
			Timestamp: time.Now(),
		},
	}

	groups := GroupByDestination(reqs)
	require.Len(t, groups, 2)

	dirKey := protocol.PartitionUrl("Directory").String()
	valKey := protocol.PartitionUrl("Validator0").String()

	require.Len(t, groups[dirKey], 2)
	require.Len(t, groups[valKey], 1)
}
