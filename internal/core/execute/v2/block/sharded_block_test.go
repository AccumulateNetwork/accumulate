// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package block

import (
	"testing"

	"github.com/stretchr/testify/require"
	"gitlab.com/accumulatenetwork/accumulate/internal/core/execute"
	"gitlab.com/accumulatenetwork/accumulate/pkg/types/messaging"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

func TestNewShardedBlock_InvalidShardCount(t *testing.T) {
	// Cannot create with non-power-of-two
	_, err := NewShardedBlock(nil, 3)
	require.Error(t, err)

	// Cannot create with 0
	_, err = NewShardedBlock(nil, 0)
	require.Error(t, err)

	// Cannot create with > 256
	_, err = NewShardedBlock(nil, 512)
	require.Error(t, err)
}

func TestNewShardedBlock_ValidShardCounts(t *testing.T) {
	for _, count := range []int{1, 2, 4, 8, 16, 32, 64, 128, 256} {
		_, err := NewShardedBlock(nil, count)
		require.NoError(t, err, "shard count %d should be valid", count)
	}
}

func TestTransactionDispatcher_DispatchBlock(t *testing.T) {
	d := execute.NewTransactionDispatcher(6) // 64 shards
	require.Equal(t, 64, d.NumShards())
}

func TestShardedBlockMetrics(t *testing.T) {
	m := ShardedBlockMetrics{
		ShardCount:       64,
		MessagesPerShard: map[int]int{0: 10, 1: 5, 2: 15},
	}
	require.Equal(t, 64, m.ShardCount)
	require.Equal(t, 10, m.MessagesPerShard[0])
}

func TestNewShardedExecutorWrapper_InvalidShardCount(t *testing.T) {
	_, err := NewShardedExecutorWrapper(nil, 3)
	require.Error(t, err)

	_, err = NewShardedExecutorWrapper(nil, 0)
	require.Error(t, err)

	_, err = NewShardedExecutorWrapper(nil, 512)
	require.Error(t, err)
}

func TestExtractShardablePrincipal_TransactionMessage(t *testing.T) {
	principal := protocol.AccountUrl("alice")
	msg := &messaging.TransactionMessage{
		Transaction: &protocol.Transaction{
			Header: protocol.TransactionHeader{
				Principal: principal,
			},
			Body: &protocol.SendTokens{},
		},
	}
	result := extractShardablePrincipal(msg)
	require.NotNil(t, result)
	require.True(t, principal.Equal(result))
}

func TestExtractShardablePrincipal_NilPrincipal(t *testing.T) {
	msg := &messaging.TransactionMessage{
		Transaction: &protocol.Transaction{
			Header: protocol.TransactionHeader{},
			Body:   &protocol.SendTokens{},
		},
	}
	result := extractShardablePrincipal(msg)
	require.Nil(t, result)
}

func TestExtractShardablePrincipal_SyntheticMessage(t *testing.T) {
	principal := protocol.AccountUrl("bob", "tokens")
	inner := &messaging.TransactionMessage{
		Transaction: &protocol.Transaction{
			Header: protocol.TransactionHeader{
				Principal: principal,
			},
			Body: &protocol.SyntheticDepositTokens{},
		},
	}
	seq := &messaging.SequencedMessage{
		Message: inner,
	}
	synMsg := &messaging.SyntheticMessage{
		Message: seq,
	}
	result := extractShardablePrincipal(synMsg)
	require.NotNil(t, result)
	require.True(t, principal.Equal(result))
}

func TestExtractShardablePrincipal_BadSyntheticMessage(t *testing.T) {
	principal := protocol.AccountUrl("carol", "tokens")
	inner := &messaging.TransactionMessage{
		Transaction: &protocol.Transaction{
			Header: protocol.TransactionHeader{
				Principal: principal,
			},
			Body: &protocol.SyntheticDepositTokens{},
		},
	}
	seq := &messaging.SequencedMessage{
		Message: inner,
	}
	badSynMsg := &messaging.BadSyntheticMessage{
		Message: seq,
	}
	result := extractShardablePrincipal(badSynMsg)
	require.NotNil(t, result)
	require.True(t, principal.Equal(result))
}

func TestExtractShardablePrincipal_SignatureMessage(t *testing.T) {
	// Signature messages should NOT be routable to shards
	sigMsg := &messaging.SignatureMessage{}
	result := extractShardablePrincipal(sigMsg)
	require.Nil(t, result)
}

func TestExtractShardablePrincipal_BlockAnchor(t *testing.T) {
	// Block anchors should NOT be routable to shards
	anchorMsg := &messaging.BlockAnchor{}
	result := extractShardablePrincipal(anchorMsg)
	require.Nil(t, result)
}

func TestExtractShardablePrincipal_SyntheticWithNilInner(t *testing.T) {
	synMsg := &messaging.SyntheticMessage{
		Message: nil,
	}
	result := extractShardablePrincipal(synMsg)
	require.Nil(t, result)
}

func TestSyntheticRoutedToCorrectShard(t *testing.T) {
	// Verify that a synthetic message targeting "bob" routes to the same shard
	// as a direct transaction targeting "bob"
	d := execute.NewTransactionDispatcher(2) // 4 shards

	principal := protocol.AccountUrl("bob", "tokens")

	// Direct transaction
	txnMsg := &messaging.TransactionMessage{
		Transaction: &protocol.Transaction{
			Header: protocol.TransactionHeader{Principal: principal},
			Body:   &protocol.SendTokens{},
		},
	}

	// Synthetic targeting the same principal
	synMsg := &messaging.SyntheticMessage{
		Message: &messaging.SequencedMessage{
			Message: &messaging.TransactionMessage{
				Transaction: &protocol.Transaction{
					Header: protocol.TransactionHeader{Principal: principal},
					Body:   &protocol.SyntheticDepositTokens{},
				},
			},
		},
	}

	txnPrincipal := extractShardablePrincipal(txnMsg)
	synPrincipal := extractShardablePrincipal(synMsg)
	require.NotNil(t, txnPrincipal)
	require.NotNil(t, synPrincipal)

	txnShard := d.RouteToShard(txnPrincipal)
	synShard := d.RouteToShard(synPrincipal)
	require.Equal(t, txnShard, synShard, "synthetic should route to same shard as direct transaction")
}
